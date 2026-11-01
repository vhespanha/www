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

func TestLoadPosts(t *testing.T) {
	fsys := fstest.MapFS{
		"posts/old.md":      {Data: []byte("---\ntitle: Old\ndate: 2025-01-02\n---\n# Heading\n")},
		"posts/new.md":      {Data: []byte("---\r\ntitle: New\r\ndate: 2026-01-02\r\n---\r\nBody\r\n")},
		"posts/ignored.txt": {Data: []byte("not a post")},
	}
	posts, err := LoadPosts(fsys, "posts/*.md")
	if err != nil {
		t.Fatal(err)
	}
	if len(posts) != 2 || posts[0].Slug != "new" || posts[1].Slug != "old" {
		t.Fatalf("got posts %+v, want new then old", posts)
	}
	if posts[1].Title != "Old" || posts[1].Date.Year() != 2025 {
		t.Errorf("got metadata %q %v", posts[1].Title, posts[1].Date)
	}
	var buf bytes.Buffer
	if err := posts[1].Body.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); !strings.Contains(got, "<h1") || !strings.Contains(got, "Heading</h1>") {
		t.Errorf("got body %q, want rendered heading", got)
	}
}

func TestLoadPostsInvalid(t *testing.T) {
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
			_, err := LoadPosts(fsys, "*.md")
			if err == nil {
				t.Fatal("got nil error")
			}
			if !strings.HasPrefix(err.Error(), "bad.md: ") {
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

	pages := []Page{{Path: "posts/a/index.html", Content: templ.Raw("<p>a</p>")}}
	if err := Build(context.Background(), out, pages); err != nil {
		t.Fatal(err)
	}

	got, err := os.ReadFile(filepath.Join(out, "posts", "a", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "<p>a</p>" {
		t.Errorf("got %q", got)
	}
	if _, err := os.Stat(filepath.Dir(stale)); !os.IsNotExist(err) {
		t.Errorf("stale output not removed: %v", err)
	}
}
