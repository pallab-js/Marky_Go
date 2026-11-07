package index

import (
	"path/filepath"
	"testing"

	"markvault/internal/domain"
)

func TestFTS5SearchAndUpsert(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s := NewService(dbPath)
	if err := s.Open(); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	root := "/tmp/vault"
	if err := s.Rebuild(root, []domain.IndexDocument{{
		Path: "a.md", AbsPath: "/tmp/vault/a.md", Title: "Alpha", Body: "hello world", Tags: []string{"go"}, MtimeNs: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	res, err := s.Search("hello", domain.SearchOptions{Limit: 10})
	if err != nil || len(res) != 1 || res[0].Path != "a.md" {
		t.Fatalf("search: res=%+v err=%v", res, err)
	}
	if err := s.UpsertByRelPath(root, "b.md", &domain.IndexDocument{Path: "b.md", AbsPath: "/tmp/vault/b.md", Title: "Beta", Body: "golang sqlite"}); err != nil {
		t.Fatal(err)
	}
	res, err = s.Search("sqlite", domain.SearchOptions{Limit: 10})
	if err != nil || len(res) != 1 || res[0].Path != "b.md" {
		t.Fatalf("search2: res=%+v err=%v", res, err)
	}
	if err := s.DeleteByRelPath(root, "b.md"); err != nil {
		t.Fatal(err)
	}
	res, err = s.Search("sqlite", domain.SearchOptions{Limit: 10})
	if err != nil || len(res) != 0 {
		t.Fatalf("search3: res=%+v err=%v", res, err)
	}
}
