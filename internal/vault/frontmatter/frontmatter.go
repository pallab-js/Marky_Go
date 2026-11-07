package frontmatter

import (
	"bufio"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type Meta struct {
	Title   string
	Tags    []string
	Aliases []string
}

type rawMeta struct {
	Title   any `yaml:"title"`
	Tags    any `yaml:"tags"`
	Aliases any `yaml:"aliases"`
}

// Parse extracts a narrow set of frontmatter fields (Decision 9).
// It is fail-safe: parse errors return zero metadata and no error.
func Parse(content string) Meta {
	fm, ok := extractYAMLBlock(content)
	if !ok {
		return Meta{}
	}
	var raw rawMeta
	if err := yaml.Unmarshal([]byte(fm), &raw); err != nil {
		return Meta{}
	}
	m := Meta{
		Title:   scalarString(raw.Title),
		Tags:    normalizeStrings(toStringList(raw.Tags)),
		Aliases: normalizeStrings(toStringList(raw.Aliases), false),
	}
	// Tags compare case-insensitively for dedupe, preserve display case.
	m.Tags = dedupeCaseInsensitive(m.Tags)
	m.Aliases = dedupeCaseInsensitive(m.Aliases)
	return m
}

func TitleOrFilename(meta Meta, relPath string) string {
	if t := strings.TrimSpace(meta.Title); t != "" {
		return t
	}
	base := filepath.Base(relPath)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func extractYAMLBlock(s string) (string, bool) {
	// Accept UTF-8 BOM.
	s = strings.TrimPrefix(s, "\ufeff")
	sc := bufio.NewScanner(strings.NewReader(s))
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	if !sc.Scan() {
		return "", false
	}
	first := strings.TrimRight(sc.Text(), "\r")
	if first != "---" {
		return "", false
	}
	var b strings.Builder
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		if line == "---" || line == "..." {
			return b.String(), true
		}
		b.WriteString(line)
		b.WriteByte('\n')
	}
	return "", false
}

func scalarString(v any) string {
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return strings.TrimSpace(s)
}

func toStringList(v any) []string {
	switch x := v.(type) {
	case nil:
		return nil
	case string:
		if strings.TrimSpace(x) == "" {
			return nil
		}
		// Allow comma-separated tags in a scalar.
		if strings.Contains(x, ",") {
			parts := strings.Split(x, ",")
			out := make([]string, 0, len(parts))
			for _, p := range parts {
				if t := strings.TrimSpace(p); t != "" {
					out = append(out, t)
				}
			}
			return out
		}
		return []string{strings.TrimSpace(x)}
	case []any:
		out := make([]string, 0, len(x))
		for _, it := range x {
			if s, ok := it.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	case []string:
		out := make([]string, 0, len(x))
		for _, s := range x {
			if strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	default:
		return nil
	}
}

func normalizeStrings(in []string, _ ...bool) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = strings.TrimSpace(s)
		s = strings.TrimPrefix(s, "#") // tolerate #tag style
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

func dedupeCaseInsensitive(in []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		k := strings.ToLower(s)
		if seen[k] {
			continue
		}
		seen[k] = true
		out = append(out, s)
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

// SearchableText returns extra text to include in FTS body (tags/aliases).
func (m Meta) SearchableText() string {
	parts := make([]string, 0, len(m.Tags)+len(m.Aliases))
	parts = append(parts, m.Tags...)
	parts = append(parts, m.Aliases...)
	return strings.Join(parts, " ")
}
