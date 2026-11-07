package index

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"markvault/internal/domain"

	_ "modernc.org/sqlite"
)

type Service struct {
	db      *sql.DB
	dbPath  string
	vaultID int64
}

func NewService(dbPath string) *Service { return &Service{dbPath: dbPath} }

func (s *Service) Open() error {
	if s.dbPath == "" {
		return errors.New("db path required")
	}
	if err := os.MkdirAll(filepath.Dir(s.dbPath), 0o755); err != nil {
		return err
	}
	db, err := sql.Open("sqlite", s.dbPath)
	if err != nil {
		return err
	}
	db.SetMaxOpenConns(1)
	s.db = db
	if _, err := db.Exec(`PRAGMA journal_mode=WAL; PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000;`); err != nil {
		return err
	}
	if err := s.migrate(); err != nil {
		return err
	}
	return s.verifyFTS5()
}

func (s *Service) Close() error {
	if s.db == nil {
		return nil
	}
	err := s.db.Close()
	s.db = nil
	return err
}

func (s *Service) migrate() error {
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS schema_migrations (version INTEGER PRIMARY KEY, applied_at TEXT NOT NULL);`,
		`CREATE TABLE IF NOT EXISTS vaults (
			id INTEGER PRIMARY KEY,
			root_path TEXT UNIQUE NOT NULL,
			last_opened_at TEXT,
			index_built_at TEXT
		);`,
		`CREATE TABLE IF NOT EXISTS notes (
			id INTEGER PRIMARY KEY,
			vault_id INTEGER NOT NULL,
			rel_path TEXT NOT NULL,
			abs_path TEXT NOT NULL,
			title TEXT,
			mtime_ns INTEGER,
			size_bytes INTEGER,
			content_hash TEXT,
			indexed_at TEXT,
			UNIQUE(vault_id, rel_path)
		);`,
		`CREATE TABLE IF NOT EXISTS note_tags (
			note_id INTEGER NOT NULL,
			tag TEXT NOT NULL,
			UNIQUE(note_id, tag)
		);`,
		// Standalone FTS5 for simplicity/correctness with manual body sync.
		`CREATE VIRTUAL TABLE IF NOT EXISTS notes_fts USING fts5(
			title, body, rel_path, note_id UNINDEXED
		);`,
		`INSERT OR IGNORE INTO schema_migrations(version, applied_at) VALUES (1, datetime('now'));`,
	}
	for _, stmt := range stmts {
		if _, err := s.db.Exec(stmt); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}
	return nil
}

func (s *Service) verifyFTS5() error {
	_, err := s.db.Exec(`CREATE VIRTUAL TABLE IF NOT EXISTS __fts5_check USING fts5(x); DROP TABLE IF EXISTS __fts5_check;`)
	if err != nil {
		return fmt.Errorf("SQLite FTS5 is required but unavailable: %w", err)
	}
	return nil
}

func (s *Service) EnsureVault(rootPath string) error {
	if s.db == nil {
		if err := s.Open(); err != nil {
			return err
		}
	}
	_, err := s.db.Exec(`INSERT INTO vaults(root_path, last_opened_at) VALUES (?, ?) ON CONFLICT(root_path) DO UPDATE SET last_opened_at=excluded.last_opened_at`, rootPath, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return err
	}
	return s.db.QueryRow(`SELECT id FROM vaults WHERE root_path=?`, rootPath).Scan(&s.vaultID)
}

func (s *Service) Rebuild(vaultRoot string, docs []domain.IndexDocument) error {
	if err := s.EnsureVault(vaultRoot); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if _, err := tx.Exec(`DELETE FROM note_tags WHERE note_id IN (SELECT id FROM notes WHERE vault_id=?)`, s.vaultID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM notes_fts WHERE note_id IN (SELECT CAST(id AS TEXT) FROM notes WHERE vault_id=?)`, s.vaultID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM notes WHERE vault_id=?`, s.vaultID); err != nil {
		return err
	}
	ins, err := tx.Prepare(`INSERT INTO notes(vault_id, rel_path, abs_path, title, mtime_ns, size_bytes, content_hash, indexed_at) VALUES (?,?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer ins.Close()
	insFTS, err := tx.Prepare(`INSERT INTO notes_fts(title, body, rel_path, note_id) VALUES (?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insFTS.Close()
	insTag, err := tx.Prepare(`INSERT OR IGNORE INTO note_tags(note_id, tag) VALUES (?, ?)`)
	if err != nil {
		return err
	}
	defer insTag.Close()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, d := range docs {
		if err := insertIndexDoc(tx, ins, insFTS, insTag, s.vaultID, d, now); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE vaults SET index_built_at=? WHERE id=?`, now, s.vaultID); err != nil {
		return err
	}
	return tx.Commit()
}

func insertIndexDoc(tx *sql.Tx, ins, insFTS, insTag *sql.Stmt, vaultID int64, d domain.IndexDocument, now string) error {
	res, err := ins.Exec(vaultID, d.Path, d.AbsPath, d.Title, d.MtimeNs, d.SizeBytes, d.ContentHash, now)
	if err != nil {
		return err
	}
	id, _ := res.LastInsertId()
	body := d.Body
	if len(d.Tags) > 0 || len(d.Aliases) > 0 {
		extra := strings.Join(append(append([]string{}, d.Tags...), d.Aliases...), " ")
		if extra != "" {
			body = body + "\n" + extra
		}
	}
	if _, err := insFTS.Exec(d.Title, body, d.Path, fmt.Sprint(id)); err != nil {
		return err
	}
	for _, tag := range d.Tags {
		if tag == "" {
			continue
		}
		if _, err := insTag.Exec(id, tag); err != nil {
			return err
		}
	}
	return nil
}

// UpsertByRelPath incrementally indexes a single .md file.
func (s *Service) UpsertByRelPath(vaultRoot, relPath string, d *domain.IndexDocument) error {
	if err := s.EnsureVault(vaultRoot); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := deleteRelPathTx(tx, s.vaultID, relPath); err != nil {
		return err
	}
	if d != nil {
		ins, err := tx.Prepare(`INSERT INTO notes(vault_id, rel_path, abs_path, title, mtime_ns, size_bytes, content_hash, indexed_at) VALUES (?,?,?,?,?,?,?,?)`)
		if err != nil {
			return err
		}
		defer ins.Close()
		insFTS, err := tx.Prepare(`INSERT INTO notes_fts(title, body, rel_path, note_id) VALUES (?,?,?,?)`)
		if err != nil {
			return err
		}
		defer insFTS.Close()
		insTag, err := tx.Prepare(`INSERT OR IGNORE INTO note_tags(note_id, tag) VALUES (?, ?)`)
		if err != nil {
			return err
		}
		defer insTag.Close()
		now := time.Now().UTC().Format(time.RFC3339)
		if err := insertIndexDoc(tx, ins, insFTS, insTag, s.vaultID, *d, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// DeleteByRelPath removes a single path (or directory prefix) from the index.
func (s *Service) DeleteByRelPath(vaultRoot, relPath string) error {
	if err := s.EnsureVault(vaultRoot); err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()
	if err := deleteRelPathTx(tx, s.vaultID, relPath); err != nil {
		return err
	}
	return tx.Commit()
}

func deleteRelPathTx(tx *sql.Tx, vaultID int64, relPath string) error {
	relPath = strings.TrimSpace(strings.ReplaceAll(relPath, "\\", "/"))
	if relPath == "" {
		return nil
	}
	// Match exact file or directory prefix.
	prefix := relPath
	if !strings.HasSuffix(prefix, "/") {
		prefix = prefix + "/"
	}
	if _, err := tx.Exec(`DELETE FROM note_tags WHERE note_id IN (SELECT id FROM notes WHERE vault_id=? AND (rel_path=? OR rel_path LIKE ?))`, vaultID, relPath, prefix+"%"); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM notes_fts WHERE note_id IN (SELECT CAST(id AS TEXT) FROM notes WHERE vault_id=? AND (rel_path=? OR rel_path LIKE ?))`, vaultID, relPath, prefix+"%"); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM notes WHERE vault_id=? AND (rel_path=? OR rel_path LIKE ?)`, vaultID, relPath, prefix+"%"); err != nil {
		return err
	}
	return nil
}

func (s *Service) Search(query string, opts domain.SearchOptions) ([]domain.SearchResult, error) {
	if s.db == nil {
		if err := s.Open(); err != nil {
			return nil, err
		}
	}
	if s.vaultID == 0 {
		return []domain.SearchResult{}, nil
	}
	q := strings.TrimSpace(query)
	if q == "" {
		return []domain.SearchResult{}, nil
	}
	limit := opts.Limit
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	match := normalizeFTSQuery(q)
	rows, err := s.db.Query(`
		SELECT n.rel_path, COALESCE(n.title,''), COALESCE(n.mtime_ns,0),
		       snippet(notes_fts, 1, '[', ']', ' … ', 16),
		       bm25(notes_fts)
		FROM notes_fts
		JOIN notes n ON CAST(n.id AS TEXT) = notes_fts.note_id
		WHERE notes_fts MATCH ? AND n.vault_id = ?
		ORDER BY bm25(notes_fts)
		LIMIT ?`, match, s.vaultID, limit)
	if err != nil {
		msg := strings.ToLower(err.Error())
		if strings.Contains(msg, "fts5") || strings.Contains(msg, "syntax") || strings.Contains(msg, "match") || strings.Contains(msg, "unterminated") {
			return nil, fmt.Errorf("invalid search query; try simpler terms")
		}
		return nil, err
	}
	defer rows.Close()
	out := []domain.SearchResult{}
	for rows.Next() {
		var r domain.SearchResult
		if err := rows.Scan(&r.Path, &r.Title, &r.MtimeNs, &r.Snippet, &r.Rank); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// normalizeFTSQuery makes plain user input safer for FTS5 MATCH.
// It preserves simple words/phrases and avoids common syntax errors from punctuation.
func normalizeFTSQuery(q string) string {
	// If user already used explicit FTS syntax characters/operators intentionally,
	// pass through as-is to avoid breaking power use.
	if strings.ContainsAny(q, `":*(){}^`) || strings.Contains(q, " AND ") || strings.Contains(q, " OR ") || strings.Contains(q, " NOT ") || strings.Contains(q, " NEAR ") {
		return q
	}
	fields := strings.Fields(q)
	if len(fields) == 0 {
		return q
	}
	terms := make([]string, 0, len(fields))
	for _, f := range fields {
		// Strip characters that commonly break FTS5 while keeping meaningful text.
		f = strings.Trim(f, ".,;!?[]<>/\\|=+-~")
		if f == "" {
			continue
		}
		f = strings.ReplaceAll(f, `"`, `""`)
		// Quote each term to treat punctuation-like chars safely.
		terms = append(terms, `"`+f+`"`)
	}
	if len(terms) == 0 {
		return q
	}
	return strings.Join(terms, " ")
}

