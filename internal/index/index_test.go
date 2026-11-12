package index

import (
	"path/filepath"
	"strings"
	"testing"

	"markvault/internal/domain"
)

func TestSearchHandlesPunctuationAndOperators(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s := NewService(dbPath)
	if err := s.Open(); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.Rebuild("/tmp/vault2", []domain.IndexDocument{
		{Path: "time.md", Title: "Time", Body: "Meeting at 10:30 about roadmap"},
		{Path: "paren.md", Title: "Paren", Body: "foo(bar) and baz"},
		{Path: "draft.md", Title: "Draft", Body: "see [draft] hello world"},
	}); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		query string
		want  int
	}{
		{"10:30", 1},
		{"foo(bar)", 1},
		{"*", 0},
		{`"unterminated`, 0},
		{"hello AND world", 1},
		{"hello AND", 1},
		{"NOT hello", 1},
		{`"hello world"`, 1},
		{"--", 0},
	}
	for _, tc := range cases {
		res, err := s.Search(tc.query, domain.SearchOptions{Limit: 10})
		if err != nil {
			t.Errorf("query %q: unexpected error: %v", tc.query, err)
			continue
		}
		if len(res) != tc.want {
			t.Errorf("query %q: got %d results, want %d (%+v)", tc.query, len(res), tc.want, res)
		}
	}
}

func TestFTS5SearchAndUpsert(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	s := NewService(dbPath)
	if err := s.Open(); err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	root := "/tmp/vault"
	if err := s.Rebuild(root, []domain.IndexDocument{{
		Path: "a.md", AbsPath: "/tmp/vault/a.md", Title: "Alpha", Body: "see [draft] hello world", Tags: []string{"go"}, MtimeNs: 1,
	}}); err != nil {
		t.Fatal(err)
	}
	res, err := s.Search("hello", domain.SearchOptions{Limit: 10})
	if err != nil || len(res) != 1 || res[0].Path != "a.md" {
		t.Fatalf("search: res=%+v err=%v", res, err)
	}
	// Highlight markers must be unambiguous: literal brackets in note text stay
	// literal, while the matched term is wrapped in the private markers.
	if !strings.Contains(res[0].Snippet, "\u0001hello\u0002") {
		t.Fatalf("expected highlight markers in snippet, got %q", res[0].Snippet)
	}
	if !strings.Contains(res[0].Snippet, "[draft]") {
		t.Fatalf("literal brackets must be preserved, got %q", res[0].Snippet)
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
