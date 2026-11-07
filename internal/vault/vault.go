package vault

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"markvault/internal/domain"
	"markvault/internal/vault/frontmatter"

	"github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	ErrNoVault      = errors.New("no vault open")
	ErrOutsideVault = errors.New("path is outside the active vault")
	ErrInvalidPath  = errors.New("invalid path")
	ErrNotMarkdown  = errors.New("only .md files are supported")
)

type Service struct {
	root string
}

func NewService() *Service { return &Service{} }

func (s *Service) OpenDialog(ctx context.Context) (string, error) {
	if ctx == nil {
		return "", errors.New("app not ready")
	}
	path, err := runtime.OpenDirectoryDialog(ctx, runtime.OpenDialogOptions{
		Title: "Open Vault Folder",
	})
	if err != nil {
		return "", err
	}
	return path, nil
}

func (s *Service) Open(path string) (domain.VaultInfo, error) {
	if strings.TrimSpace(path) == "" {
		return domain.VaultInfo{}, ErrInvalidPath
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return domain.VaultInfo{}, err
	}
	abs, err = filepath.EvalSymlinks(abs)
	if err != nil {
		return domain.VaultInfo{}, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return domain.VaultInfo{}, err
	}
	if !st.IsDir() {
		return domain.VaultInfo{}, errors.New("vault path must be a directory")
	}
	s.root = abs
	return domain.VaultInfo{
		RootPath:     abs,
		Name:         filepath.Base(abs),
		LastOpenedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (s *Service) Root() string { return s.root }

func (s *Service) Tree() ([]*domain.TreeNode, error) {
	if s.root == "" {
		return nil, ErrNoVault
	}
	return s.readTree(s.root, "")
}

func (s *Service) readTree(absDir, relDir string) ([]*domain.TreeNode, error) {
	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, err
	}
	nodes := make([]*domain.TreeNode, 0, len(entries))
	for _, e := range entries {
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		abs := filepath.Join(absDir, name)
		rel := name
		if relDir != "" {
			rel = filepath.ToSlash(filepath.Join(relDir, name))
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		// Decision 12: do not follow symlinks.
		if info.Mode()&os.ModeSymlink != 0 {
			continue
		}
		if e.IsDir() {
			children, err := s.readTree(abs, rel)
			if err != nil {
				children = []*domain.TreeNode{}
			}
			nodes = append(nodes, &domain.TreeNode{
				Name:     name,
				Path:     rel,
				AbsPath:  abs,
				IsDir:    true,
				Children: children,
			})
			continue
		}
		if !isMarkdown(name) {
			continue
		}
		mt := info.ModTime().UTC().Format(time.RFC3339Nano)
		nodes = append(nodes, &domain.TreeNode{
			Name:    name,
			Path:    rel,
			AbsPath: abs,
			IsDir:   false,
			ModTime: mt,
			Size:    info.Size(),
		})
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].IsDir != nodes[j].IsDir {
			return nodes[i].IsDir
		}
		return strings.ToLower(nodes[i].Name) < strings.ToLower(nodes[j].Name)
	})
	return nodes, nil
}

func (s *Service) ReadNote(relPath string) (domain.NoteDocument, error) {
	abs, _, err := s.resolveNotePath(relPath)
	if err != nil {
		return domain.NoteDocument{}, err
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return domain.NoteDocument{}, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return domain.NoteDocument{}, err
	}
	content, ending := normalizeLoadedContent(string(b))
	h := sha256.Sum256([]byte(content))
	return domain.NoteDocument{
		Path:        filepath.ToSlash(relPath),
		AbsPath:     abs,
		Content:     content,
		MtimeNs:     st.ModTime().UnixNano(),
		SizeBytes:   st.Size(),
		ContentHash: hex.EncodeToString(h[:]),
		LineEnding:  ending,
	}, nil
}

// SaveNote writes content to disk with conflict detection (Decision 8,17).
// It compares current disk hash/mtime to the base version supplied by the client.
// If the file changed externally and the base does not match, it returns a conflict
// unless forceOverwrite is explicitly true.
func (s *Service) SaveNote(relPath, content, baseHash string, baseMtimeNs int64, lineEnding string, forceOverwrite bool) (domain.SaveResult, error) {
	abs, rel, err := s.resolveNotePath(relPath)
	if err != nil {
		return domain.SaveResult{}, err
	}
	// Check current disk state.
	if st, err := os.Stat(abs); err == nil {
		b, err := os.ReadFile(abs)
		if err != nil {
			return domain.SaveResult{}, err
		}
		diskContent, _ := normalizeLoadedContent(string(b))
		diskHash := hashContent(diskContent)
		changed := diskHash != baseHash || st.ModTime().UnixNano() != baseMtimeNs
		baseKnown := baseHash != "" || baseMtimeNs != 0
		if changed && baseKnown && !forceOverwrite {
			return domain.SaveResult{
				Conflict:     true,
				ConflictKind: domain.ConflictExternalChange,
				Message:      "File changed on disk since it was opened. Choose reload, keep your version, or save a copy.",
			}, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return domain.SaveResult{}, err
	}

	if lineEnding != "\r\n" {
		lineEnding = "\n"
	}
	diskContent := content
	if lineEnding == "\r\n" {
		diskContent = strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\n", "\r\n")
	}
	if err := atomicWriteFile(abs, []byte(diskContent)); err != nil {
		return domain.SaveResult{}, err
	}
	st, err := os.Stat(abs)
	if err != nil {
		return domain.SaveResult{}, err
	}
	h := hashContent(content)
	doc := &domain.NoteDocument{
		Path:        filepath.ToSlash(rel),
		AbsPath:     abs,
		Content:     content,
		MtimeNs:     st.ModTime().UnixNano(),
		SizeBytes:   st.Size(),
		ContentHash: h,
		LineEnding:  lineEnding,
	}
	return domain.SaveResult{Saved: true, Document: doc}, nil
}

// SaveConflictCopy saves content as "name (conflict YYYYMMDD-HHMMSS).md" beside the original.
func (s *Service) SaveConflictCopy(relPath, content, lineEnding string) (domain.SaveResult, error) {
	abs, _, err := s.resolveNotePath(relPath)
	if err != nil {
		return domain.SaveResult{}, err
	}
	if lineEnding != "\r\n" {
		lineEnding = "\n"
	}
	ext := filepath.Ext(abs)
	base := strings.TrimSuffix(abs, ext)
	stamp := time.Now().Format("20060102-150405")
	copyAbs := fmt.Sprintf("%s (conflict %s)%s", base, stamp, ext)
	diskContent := content
	if lineEnding == "\r\n" {
		diskContent = strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\n", "\r\n")
	}
	if err := atomicWriteFile(copyAbs, []byte(diskContent)); err != nil {
		return domain.SaveResult{}, err
	}
	relCopy, err := filepath.Rel(s.root, copyAbs)
	if err != nil {
		relCopy = filepath.Base(copyAbs)
	}
	st, _ := os.Stat(copyAbs)
	doc := &domain.NoteDocument{
		Path:        filepath.ToSlash(relCopy),
		AbsPath:     copyAbs,
		Content:     content,
		ContentHash: hashContent(content),
		LineEnding:  lineEnding,
	}
	if st != nil {
		doc.MtimeNs = st.ModTime().UnixNano()
		doc.SizeBytes = st.Size()
	}
	return domain.SaveResult{Saved: true, Document: doc, Message: "Saved conflict copy"}, nil
}

func atomicWriteFile(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".markvault-save-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() { _ = os.Remove(tmpName) }()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, path)
}

func hashContent(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])
}

// CollectIndexDocument builds an index document for a known .md path.
func (s *Service) CollectIndexDocument(relPath string) (*domain.IndexDocument, error) {
	if s.root == "" {
		return nil, ErrNoVault
	}
	abs, rel, err := s.resolveNotePath(relPath)
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return nil, err
	}
	if info.IsDir() || info.Mode()&os.ModeSymlink != 0 || !isMarkdown(abs) {
		return nil, ErrInvalidPath
	}
	b, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	content, _ := normalizeLoadedContent(string(b))
	meta := frontmatter.Parse(content)
	h := sha256.Sum256([]byte(content))
	return &domain.IndexDocument{
		Path:        rel,
		AbsPath:     abs,
		Title:       frontmatter.TitleOrFilename(meta, rel),
		Body:        content,
		Tags:        meta.Tags,
		Aliases:     meta.Aliases,
		MtimeNs:     info.ModTime().UnixNano(),
		SizeBytes:   info.Size(),
		ContentHash: hex.EncodeToString(h[:]),
	}, nil
}

// CollectIndexDocuments scans the vault for .md files and prepares derived index docs.
// Read-only frontmatter parse is fail-safe (Decision 9).
func (s *Service) CollectIndexDocuments() ([]domain.IndexDocument, error) {
	if s.root == "" {
		return nil, ErrNoVault
	}
	docs := make([]domain.IndexDocument, 0, 256)
	err := filepath.WalkDir(s.root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return nil
		}
		if info.Mode()&os.ModeSymlink != 0 {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		if !isMarkdown(name) {
			return nil
		}
		rel, err := filepath.Rel(s.root, path)
		if err != nil {
			return nil
		}
		rel = filepath.ToSlash(rel)
		b, err := os.ReadFile(path)
		if err != nil {
			return nil
		}
		content, _ := normalizeLoadedContent(string(b))
		meta := frontmatter.Parse(content)
		h := sha256.Sum256([]byte(content))
		docs = append(docs, domain.IndexDocument{
			Path:        rel,
			AbsPath:     path,
			Title:       frontmatter.TitleOrFilename(meta, rel),
			Body:        content,
			Tags:        meta.Tags,
			Aliases:     meta.Aliases,
			MtimeNs:     info.ModTime().UnixNano(),
			SizeBytes:   info.Size(),
			ContentHash: hex.EncodeToString(h[:]),
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(docs, func(i, j int) bool { return docs[i].Path < docs[j].Path })
	return docs, nil
}

// ResolveAbsPath returns the absolute path for a vault-relative markdown path.
func (s *Service) ResolveAbsPath(relPath string) (abs string, rel string, err error) {
	return s.resolveNotePath(relPath)
}

func (s *Service) resolveAnyPath(relPath string, wantDir bool) (abs string, rel string, err error) {
	if s.root == "" {
		return "", "", ErrNoVault
	}
	rel = filepath.ToSlash(filepath.Clean(strings.TrimSpace(relPath)))
	if rel == "." || rel == "" || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return "", "", ErrInvalidPath
	}
	if hasDotSegment(rel) {
		return "", "", ErrOutsideVault
	}
	abs = filepath.Join(s.root, filepath.FromSlash(rel))
	rootReal, err := filepath.EvalSymlinks(s.root)
	if err != nil {
		return "", "", err
	}
	absReal, err := resolvePathWithinRoot(rootReal, abs)
	if err != nil {
		return "", "", err
	}
	if info, err := os.Lstat(absReal); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", "", ErrOutsideVault
		}
		if wantDir && !info.IsDir() {
			return "", "", ErrInvalidPath
		}
		if !wantDir && info.IsDir() {
			return "", "", ErrInvalidPath
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", "", err
	}
	return absReal, rel, nil
}

func (s *Service) resolveNotePath(relPath string) (abs string, rel string, err error) {
	if s.root == "" {
		return "", "", ErrNoVault
	}
	rel = filepath.ToSlash(filepath.Clean(strings.TrimSpace(relPath)))
	if rel == "." || rel == "" || strings.HasPrefix(rel, "../") || filepath.IsAbs(rel) {
		return "", "", ErrInvalidPath
	}
	if hasDotSegment(rel) {
		return "", "", ErrOutsideVault
	}
	if !isMarkdown(rel) {
		return "", "", ErrNotMarkdown
	}
	abs = filepath.Join(s.root, filepath.FromSlash(rel))
	rootReal, err := filepath.EvalSymlinks(s.root)
	if err != nil {
		return "", "", err
	}
	absReal, err := resolvePathWithinRoot(rootReal, abs)
	if err != nil {
		return "", "", err
	}
	// If target exists and is symlink, reject (Decision 12).
	if info, err := os.Lstat(absReal); err == nil {
		if info.Mode()&os.ModeSymlink != 0 {
			return "", "", ErrOutsideVault
		}
		if info.IsDir() {
			return "", "", ErrInvalidPath
		}
	} else if !errors.Is(err, fs.ErrNotExist) {
		return "", "", err
	}
	return absReal, rel, nil
}

// resolvePathWithinRoot resolves symlinks for the nearest existing ancestor and
// enforces that the resulting path stays within rootReal. Missing leaf segments
// are allowed for create operations; existing symlink segments are resolved.
func resolvePathWithinRoot(rootReal, abs string) (string, error) {
	cur := filepath.Clean(abs)
	suffix := make([]string, 0, 4)
	for {
		if rel, err := filepath.Rel(rootReal, cur); err == nil {
			if strings.HasPrefix(rel, "..") {
				return "", ErrOutsideVault
			}
		}
		if _, err := os.Lstat(cur); err == nil {
			realCur, err := filepath.EvalSymlinks(cur)
			if err != nil {
				return "", err
			}
			relCheck, err := filepath.Rel(rootReal, realCur)
			if err != nil || strings.HasPrefix(relCheck, "..") {
				return "", ErrOutsideVault
			}
			out := realCur
			for i := len(suffix) - 1; i >= 0; i-- {
				out = filepath.Join(out, suffix[i])
			}
			relCheck, err = filepath.Rel(rootReal, out)
			if err != nil || strings.HasPrefix(relCheck, "..") {
				return "", ErrOutsideVault
			}
			return out, nil
		} else if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			return "", ErrOutsideVault
		}
		suffix = append(suffix, filepath.Base(cur))
		cur = parent
	}
}


func (s *Service) CreateNote(relPath, content string) (domain.NoteDocument, error) {
	if !isMarkdown(relPath) {
		return domain.NoteDocument{}, ErrNotMarkdown
	}
	abs, rel, err := s.resolveNotePath(relPath)
	if err != nil {
		return domain.NoteDocument{}, err
	}
	if _, err := os.Lstat(abs); err == nil {
		return domain.NoteDocument{}, fmt.Errorf("file already exists")
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return domain.NoteDocument{}, err
	}
	res, err := s.SaveNote(rel, content, "", 0, "\n", true)
	if err != nil {
		return domain.NoteDocument{}, err
	}
	if res.Document == nil {
		return domain.NoteDocument{}, fmt.Errorf("failed to create note")
	}
	return *res.Document, nil
}

func (s *Service) CreateFolder(relPath string) error {
	abs, _, err := s.resolveAnyPath(relPath, true)
	if err != nil {
		return err
	}
	return os.MkdirAll(abs, 0o755)
}

func (s *Service) RenamePath(oldRel, newRel string) error {
	if s.root == "" {
		return ErrNoVault
	}
	oldAbs, _, err := s.resolveAnyPath(oldRel, false)
	if err != nil {
		// try directory
		oldAbs, _, err = s.resolveAnyPath(oldRel, true)
		if err != nil {
			return err
		}
	}
	info, err := os.Lstat(oldAbs)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrOutsideVault
	}
	if !info.IsDir() && !isMarkdown(oldRel) {
		return ErrNotMarkdown
	}
	if !info.IsDir() && !isMarkdown(newRel) {
		return ErrNotMarkdown
	}
	newAbs, _, err := s.resolveAnyPath(newRel, info.IsDir())
	if err != nil {
		return err
	}
	if _, err := os.Lstat(newAbs); err == nil {
		return fmt.Errorf("target already exists")
	}
	if err := os.MkdirAll(filepath.Dir(newAbs), 0o755); err != nil {
		return err
	}
	return os.Rename(oldAbs, newAbs)
}

func (s *Service) DeletePath(relPath string) error {
	// Resolve without requiring extension type for directories.
	abs, _, err := s.resolveAnyPath(relPath, false)
	if err != nil {
		abs, _, err = s.resolveAnyPath(relPath, true)
		if err != nil {
			// Also allow markdown-resolve style path safety.
			return err
		}
	}
	info, err := os.Lstat(abs)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return ErrOutsideVault
	}
	return MoveToTrash(abs)
}

func hasDotSegment(rel string) bool {
	for _, p := range strings.Split(filepath.ToSlash(rel), "/") {
		if strings.HasPrefix(p, ".") {
			return true
		}
	}
	return false
}

func isMarkdown(name string) bool {
	return strings.EqualFold(filepath.Ext(name), ".md")
}

func normalizeLoadedContent(s string) (content string, lineEnding string) {
	// Detect dominant line ending (Decision 17): preserve detected style.
	crlf := strings.Count(s, "\r\n")
	lf := strings.Count(strings.ReplaceAll(s, "\r\n", ""), "\n")
	if crlf > 0 && crlf >= lf {
		lineEnding = "\r\n"
		// Normalize internal representation to \n for editor; remember original style.
		content = strings.ReplaceAll(s, "\r\n", "\n")
		return content, lineEnding
	}
	if lf > 0 || crlf == 0 {
		lineEnding = "\n"
		return s, lineEnding
	}
	return "\n", "\n"
}
