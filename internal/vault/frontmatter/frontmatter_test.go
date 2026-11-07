package frontmatter

import "testing"

func TestParseTagsAliasesTitle(t *testing.T) {
	doc := "---\ntitle: Hello\ntags: [a, B, a]\naliases: hi\n---\nBody"
	m := Parse(doc)
	if m.Title != "Hello" {
		t.Fatalf("title: %q", m.Title)
	}
	if len(m.Tags) != 2 || m.Tags[0] != "a" || m.Tags[1] != "B" {
		t.Fatalf("tags: %#v", m.Tags)
	}
	if len(m.Aliases) != 1 || m.Aliases[0] != "hi" {
		t.Fatalf("aliases: %#v", m.Aliases)
	}
}

func TestParseFailSafe(t *testing.T) {
	m := Parse("---\n: bad yaml [\n---\nbody")
	if m.Title != "" || len(m.Tags) != 0 || len(m.Aliases) != 0 {
		t.Fatalf("expected empty meta, got %#v", m)
	}
}
