package ssg

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/a-h/templ"
)

func TestLoadNotes(t *testing.T) {
	fsys := fstest.MapFS{
		"notes/old.md":      {Data: []byte("---\ntitle: Old\ndate: 2025-01-02\n---\n# Heading\n\nNote[^1]\n\n[^1]: Text\n\n<aside>raw</aside>\n")},
		"notes/new.md":      {Data: []byte("---\r\ntitle: New\r\ndate: 2026-01-02\r\n---\r\nBody\r\n")},
		"notes/ignored.txt": {Data: []byte("not a note")},
	}
	notes, err := LoadNotes(fsys, "notes/*.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 2 || notes[0].Slug != "new" || notes[1].Slug != "old" {
		t.Fatalf("got notes %+v, want new then old", notes)
	}
	if notes[1].Title != "Old" || notes[1].Date.Year() != 2025 {
		t.Errorf("got metadata %q %v", notes[1].Title, notes[1].Date)
	}
	if notes[1].Minutes != 1 {
		t.Errorf("got minutes %d", notes[1].Minutes)
	}
	var buf bytes.Buffer
	if err := notes[1].Body.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Heading</h1>", `class="footnotes"`, "<aside>raw</aside>"} {
		if got := buf.String(); !strings.Contains(got, want) {
			t.Errorf("got body %q, want %q", got, want)
		}
	}
}

func TestLoadNotesInvalid(t *testing.T) {
	tests := map[string]string{
		"no front matter": "# Heading\n",
		"unterminated":    "---\ntitle: A\ndate: 2026-01-02\n",
		"empty":           "---\n---\nBody\n",
		"no title":        "---\ndate: 2026-01-02\n---\n",
		"no date":         "---\ntitle: A\n---\n",
		"bad yaml":        "---\ntitle: [\n---\n",
	}
	for name, src := range tests {
		t.Run(name, func(t *testing.T) {
			fsys := fstest.MapFS{"bad.md": {Data: []byte(src)}}
			_, err := LoadNotes(fsys, "*.md")
			if err == nil {
				t.Fatal("got nil error")
			}
			if !strings.HasPrefix(err.Error(), "bad.md: ") {
				t.Errorf("error %q does not name the file", err)
			}
		})
	}
}

func TestLoadNotesTranslations(t *testing.T) {
	fsys := fstest.MapFS{
		"a.md":    {Data: []byte("---\ntitle: A\ndate: 2026-01-02\n---\nBody\n")},
		"a.pt.md": {Data: []byte("---\ntitle: A em pt\n---\nCorpo\n")},
		"b.md":    {Data: []byte("---\ntitle: B\ndate: 2025-01-02\n---\nBody\n")},
		"b.pt.md": {Data: []byte("---\ntitle: B em pt\ndate: 2025-01-02\n---\nCorpo\n")},
		"c.md":    {Data: []byte("---\ntitle: C\ndate: 2024-01-02\n---\nBody\n")},
	}
	notes, err := LoadNotes(fsys, "*.md", "pt")
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) != 3 {
		t.Fatalf("got %d notes, want the 3 originals", len(notes))
	}
	a, ok := notes[0].Translations["pt"]
	if !ok {
		t.Fatal("translation of a not attached")
	}
	if a.Title != "A em pt" || a.Slug != "a" || !a.Date.Equal(notes[0].Date) {
		t.Errorf("got translation %q %q %v", a.Title, a.Slug, a.Date)
	}
	if notes[2].Translations != nil {
		t.Errorf("got translations %v on an untranslated note", notes[2].Translations)
	}
}

func TestLoadNotesInvalidTranslations(t *testing.T) {
	const original = "---\ntitle: A\ndate: 2026-01-02\n---\n"
	tests := map[string]fstest.MapFS{
		"no original": {
			"a.pt.md": {Data: []byte("---\ntitle: A\n---\n")},
		},
		"unknown language": {
			"a.md":    {Data: []byte(original)},
			"a.es.md": {Data: []byte("---\ntitle: A\n---\n")},
		},
		"different date": {
			"a.md":    {Data: []byte(original)},
			"a.pt.md": {Data: []byte("---\ntitle: A\ndate: 2026-01-03\n---\n")},
		},
		"no title": {
			"a.md":    {Data: []byte(original)},
			"a.pt.md": {Data: []byte("---\ndate: 2026-01-02\n---\n")},
		},
	}
	for name, fsys := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := LoadNotes(fsys, "*.md", "pt")
			if err == nil {
				t.Fatal("got nil error")
			}
			if !strings.Contains(err.Error(), ".md: ") {
				t.Errorf("error %q does not name the file", err)
			}
		})
	}
}

func TestBuild(t *testing.T) {
	out := t.TempDir()
	stale := filepath.Join(out, "stale", "index.html")
	if err := os.MkdirAll(filepath.Dir(stale), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stale, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	pages := []Page{{Path: "notes/a/index.html", Content: templ.Raw("<p>a</p>")}}
	assets := fstest.MapFS{"assets/site.css": {Data: []byte("a{}")}}
	if err := Build(context.Background(), out, pages, assets); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(out, "notes", "a", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "<p>a</p>" {
		t.Errorf("got %q", got)
	}
	if _, err := os.Stat(filepath.Join(out, "assets", "site.css")); err != nil {
		t.Errorf("asset not copied: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(stale)); !os.IsNotExist(err) {
		t.Errorf("stale output not removed: %v", err)
	}
}
