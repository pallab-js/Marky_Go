package vault

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveNotePathConfines(t *testing.T) {
	root := t.TempDir()
	s := NewService()
	if _, err := s.Open(root); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ok.md"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.resolveNotePath("../escape.md"); err == nil {
		t.Fatal("expected escape rejection")
	}
	if _, _, err := s.resolveNotePath(".hidden/note.md"); err == nil {
		t.Fatal("expected dot-segment rejection")
	}
	abs, rel, err := s.resolveNotePath("ok.md")
	if err != nil || rel != "ok.md" || abs == "" {
		t.Fatalf("expected ok.md, got abs=%q rel=%q err=%v", abs, rel, err)
	}
	if runtime.GOOS != "" { // ensure test compiles everywhere
		if _, _, err := s.resolveNotePath("/etc/passwd.md"); err == nil {
			t.Fatal("expected absolute path rejection")
		}
	}
}

func TestSaveConflictDetection(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "n.md")
	if err := os.WriteFile(p, []byte("v1"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewService()
	if _, err := s.Open(root); err != nil {
		t.Fatal(err)
	}
	doc, err := s.ReadNote("n.md")
	if err != nil {
		t.Fatal(err)
	}
	// External change.
	if err := os.WriteFile(p, []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	res, err := s.SaveNote("n.md", "local edit", doc.ContentHash, doc.MtimeNs, doc.LineEnding, false)
	if err != nil {
		t.Fatal(err)
	}
	if !res.Conflict || res.Saved {
		t.Fatalf("expected conflict, got %+v", res)
	}
	res, err = s.SaveNote("n.md", "local edit", doc.ContentHash, doc.MtimeNs, doc.LineEnding, true)
	if err != nil {
		t.Fatal(err)
	}
	if res.Conflict || !res.Saved {
		t.Fatalf("expected forced save, got %+v", res)
	}
	b, _ := os.ReadFile(p)
	if string(b) != "local edit" {
		t.Fatalf("content: %q", string(b))
	}
}

func TestSaveConflictCopyAndLineEnding(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "note.md")
	if err := os.WriteFile(p, []byte("a\r\nb\r\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := NewService()
	if _, err := s.Open(root); err != nil {
		t.Fatal(err)
	}
	doc, err := s.ReadNote("note.md")
	if err != nil {
		t.Fatal(err)
	}
	if doc.LineEnding != "\r\n" {
		t.Fatalf("expected CRLF, got %q", doc.LineEnding)
	}
	res, err := s.SaveConflictCopy("note.md", "x\ny\n", "\r\n")
	if err != nil || !res.Saved || res.Document == nil {
		t.Fatalf("conflict copy: %+v err=%v", res, err)
	}
	b, err := os.ReadFile(res.Document.AbsPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "x\r\ny\r\n" {
		t.Fatalf("unexpected copy content: %q", string(b))
	}
}

func TestCreateRenameDeleteMarkdownOnly(t *testing.T) {
	root := t.TempDir()
	s := NewService()
	if _, err := s.Open(root); err != nil {
		t.Fatal(err)
	}
	if err := s.CreateFolder("Notes/Sub"); err != nil {
		t.Fatal(err)
	}
	doc, err := s.CreateNote("Notes/Sub/Idea.md", "# Idea\n")
	if err != nil || doc.Path != "Notes/Sub/Idea.md" {
		t.Fatalf("create note: %+v err=%v", doc, err)
	}
	if _, err := s.CreateNote("Notes/Sub/bad.txt", "x"); err == nil {
		t.Fatal("expected .txt rejection")
	}
	if err := s.RenamePath("Notes/Sub/Idea.md", "Notes/Sub/Idea2.md"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ReadNote("Notes/Sub/Idea2.md"); err != nil {
		t.Fatal(err)
	}
}

