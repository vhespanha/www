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
		"spaced closing":  "---\ntitle: A\ndate: 2026-01-02\n--- \nFirst\n\n---\n\nSecond\n",
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

func TestLoadNotesInvalidSlug(t *testing.T) {
	src := []byte("---\ntitle: A\ndate: 2026-01-02\n---\n")
	for _, name := range []string{"c#-tips.md", "Upper.md", "a b.md"} {
		_, err := LoadNotes(fstest.MapFS{name: {Data: src}}, "*.md")
		if err == nil || !strings.HasPrefix(err.Error(), name+": ") {
			t.Errorf("%s: got error %v", name, err)
		}
	}
}

func TestBuildRefusesProtectedDirs(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	wd := filepath.Join(t.TempDir(), "wd")
	if err := os.Mkdir(wd, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(wd)
	keep := filepath.Join(wd, "keep")
	if err := os.WriteFile(keep, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	for _, out := range []string{".", "..", home} {
		if err := Build(context.Background(), out, nil, fstest.MapFS{}); err == nil {
			t.Errorf("Build(%q): got nil error", out)
		}
	}
	if _, err := os.Stat(keep); err != nil {
		t.Errorf("working directory was cleared: %v", err)
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
