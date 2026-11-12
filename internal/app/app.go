package app

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"markvault/internal/domain"
	"markvault/internal/index"
	"markvault/internal/settings"
	"markvault/internal/vault"
	"markvault/internal/watcher"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

type App struct {
	ctx      context.Context
	settings *settings.Service
	vault    *vault.Service
	index    *index.Service
	watcher  *watcher.Service
}

func New() *App {
	s := settings.NewService()
	dbPath, _ := s.DBPath()
	a := &App{
		settings: s,
		vault:    vault.NewService(),
		index:    index.NewService(dbPath),
	}
	a.watcher = watcher.NewService(func(ev watcher.Event) {
		if a.ctx == nil {
			return
		}
		root := a.vault.Root()
		if root != "" {
			rel, err := filepath.Rel(root, ev.Path)
			inside := err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
			if inside && strings.HasSuffix(strings.ToLower(rel), ".md") {
				a.indexPath(filepath.ToSlash(rel))
			} else {
				_ = a.RebuildIndex()
			}
		}
		wailsRuntime.EventsEmit(a.ctx, "vault:changed", map[string]string{"path": ev.Path, "cause": ev.Cause})
	})

	return a
}

func (a *App) Startup(ctx context.Context) {
	a.ctx = ctx
	if _, err := a.settings.EnsureDataDir(); err != nil {
		_ = err
	}
}

func (a *App) Shutdown(ctx context.Context) {
	_ = ctx
	if a.watcher != nil {
		a.watcher.Stop()
	}
	if a.index != nil {
		_ = a.index.Close()
	}
}

func (a *App) GetSettings() (domain.Settings, error) {
	return a.settings.Load()
}

func (a *App) UpdateSettings(s domain.Settings) (domain.Settings, error) {
	return a.settings.Save(s)
}

func (a *App) Health() string {
	return "ok"
}

func (a *App) AppInfo() map[string]string {
	db, _ := a.settings.DBPath()
	cfg, _ := a.settings.ConfigPath()
	return map[string]string{
		"name":       "Markvault",
		"dbPath":     db,
		"configPath": cfg,
	}
}

func (a *App) OpenVaultDialog() (string, error) {
	return a.vault.OpenDialog(a.ctx)
}

func (a *App) OpenVault(path string) (domain.VaultInfo, error) {
	info, err := a.vault.Open(path)
	if err != nil {
		return domain.VaultInfo{}, err
	}
	s, err := a.settings.Load()
	if err == nil {
		s.LastActiveVault = info.RootPath
		s.RecentVaults = prependUnique(s.RecentVaults, info.RootPath, 10)
		_, _ = a.settings.Save(s)
	}
	if err := a.RebuildIndex(); err != nil {
		_ = err
	}
	if a.watcher != nil {
		_ = a.watcher.Start(info.RootPath)
	}
	return info, nil
}

func (a *App) GetTree() ([]*domain.TreeNode, error) {
	return a.vault.Tree()
}

func (a *App) ReadNote(relPath string) (domain.NoteDocument, error) {
	return a.vault.ReadNote(relPath)
}

func (a *App) WriteNote(relPath, content, baseHash string, baseMtimeNs int64, lineEnding string, forceOverwrite bool) (domain.SaveResult, error) {
	if a.watcher != nil {
		if abs, _, err := a.vault.ResolveAbsPath(relPath); err == nil {
			a.watcher.MarkSelfWrite(abs)
		}
	}
	res, err := a.vault.SaveNote(relPath, content, baseHash, baseMtimeNs, lineEnding, forceOverwrite)
	if err != nil {
		return domain.SaveResult{}, err
	}
	if res.Saved && res.Document != nil {
		go a.indexPath(res.Document.Path)
	}
	return res, nil
}

func (a *App) SaveConflictCopy(relPath, content, lineEnding string) (domain.SaveResult, error) {
	res, err := a.vault.SaveConflictCopy(relPath, content, lineEnding)
	if err != nil {
		return domain.SaveResult{}, err
	}
	go a.indexPath(relPath)
	if res.Document != nil {
		go a.indexPath(res.Document.Path)
	}
	return res, nil
}

func (a *App) indexPath(relPath string) {
	root := a.vault.Root()
	if root == "" {
		return
	}
	doc, err := a.vault.CollectIndexDocument(relPath)
	if err != nil {
		_ = a.index.DeleteByRelPath(root, relPath)
		return
	}
	if err := a.index.UpsertByRelPath(root, relPath, doc); err != nil {
		_ = a.RebuildIndex()
	}
}

func (a *App) RebuildIndex() error {
	root := a.vault.Root()
	if root == "" {
		return fmt.Errorf("no vault open")
	}
	docs, err := a.vault.CollectIndexDocuments()
	if err != nil {
		return err
	}
	return a.index.Rebuild(root, docs)
}

func (a *App) Search(query string, opts domain.SearchOptions) ([]domain.SearchResult, error) {
	if a.vault.Root() == "" {
		return []domain.SearchResult{}, nil
	}
	_ = a.index.EnsureVault(a.vault.Root())
	return a.index.Search(query, opts)
}

func (a *App) CreateNote(relPath, content string) (domain.NoteDocument, error) {
	doc, err := a.vault.CreateNote(relPath, content)
	if err != nil {
		return domain.NoteDocument{}, err
	}
	go func() {
		a.indexPath(doc.Path)
		wailsRuntime.EventsEmit(a.ctx, "vault:changed", map[string]string{"path": relPath, "cause": "create"})
	}()
	return doc, nil
}

func (a *App) CreateFolder(relPath string) error {
	return a.vault.CreateFolder(relPath)
}

func (a *App) RenamePath(oldRel, newRel string) error {

	if err := a.vault.RenamePath(oldRel, newRel); err != nil {
		return err
	}
	go func() {
		root := a.vault.Root()
		if root != "" {
			_ = a.index.DeleteByRelPath(root, oldRel)
		}
		a.indexPath(newRel)
		wailsRuntime.EventsEmit(a.ctx, "vault:changed", map[string]string{"path": newRel, "cause": "rename"})
	}()
	return nil
}

func (a *App) DeletePath(relPath string) error {
	if err := a.vault.DeletePath(relPath); err != nil {
		return err
	}
	go func() {
		root := a.vault.Root()
		if root != "" {
			_ = a.index.DeleteByRelPath(root, relPath)
		}
		wailsRuntime.EventsEmit(a.ctx, "vault:changed", map[string]string{"path": relPath, "cause": "delete"})
	}()
	return nil
}

func prependUnique(items []string, v string, max int) []string {
	out := make([]string, 0, len(items)+1)
	out = append(out, v)
	for _, it := range items {
		if it == v || it == "" {
			continue
		}
		out = append(out, it)
	}
	if max > 0 && len(out) > max {
		out = out[:max]
	}
	return out
}
