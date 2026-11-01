// Package ssg implements the static site generator.
package ssg

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"go.yaml.in/yaml/v3"
)

// A Page is a component rendered to a file in the output directory.
type Page struct {
	// Path is the slash-separated output file, relative to the output
	// directory. For example, "posts/hello/index.html".
	Path    string
	Content templ.Component
}

// Build replaces the contents of the directory out with the rendered pages.
func Build(ctx context.Context, out string, pages []Page) error {
	if err := clearDir(out); err != nil {
		return err
	}
	for _, p := range pages {
		var buf bytes.Buffer
		if err := p.Content.Render(ctx, &buf); err != nil {
			return fmt.Errorf("render %s: %w", p.Path, err)
		}
		name := filepath.Join(out, filepath.FromSlash(p.Path))
		if err := os.MkdirAll(filepath.Dir(name), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(name, buf.Bytes(), 0o644); err != nil {
			return err
		}
	}
	return nil
}

// clearDir empties dir, creating it if needed. It keeps dir itself so that
// anything serving or watching it stays attached across rebuilds.
func clearDir(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := os.RemoveAll(filepath.Join(dir, e.Name())); err != nil {
			return err
		}
	}
	return nil
}

// A Post is a post parsed from a markdown file.
type Post struct {
	// Slug is the name of the source file without its extension.
	Slug  string
	Title string
	Date  time.Time
	// Body is the post's markdown rendered to HTML.
	Body templ.Component
}

// LoadPosts parses the markdown files in fsys matching the glob pattern and
// returns them newest first.
//
// Each file must start with a YAML front matter block setting title and date:
//
//	---
//	title: Hello
//	date: 2026-11-01
//	---
func LoadPosts(fsys fs.FS, pattern string) ([]Post, error) {
	names, err := fs.Glob(fsys, pattern)
	if err != nil {
		return nil, err
	}
	posts := make([]Post, 0, len(names))
	for _, name := range names {
		src, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		post, err := parsePost(src)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		post.Slug = strings.TrimSuffix(path.Base(name), path.Ext(name))
		posts = append(posts, post)
	}
	slices.SortFunc(posts, func(a, b Post) int {
		return cmp.Or(b.Date.Compare(a.Date), cmp.Compare(a.Slug, b.Slug))
	})
	return posts, nil
}

var markdown = goldmark.New(goldmark.WithExtensions(extension.GFM))

func parsePost(src []byte) (Post, error) {
	front, body, err := splitFrontMatter(src)
	if err != nil {
		return Post{}, err
	}
	var meta struct {
		Title string    `yaml:"title"`
		Date  time.Time `yaml:"date"`
	}
	if err := yaml.Unmarshal(front, &meta); err != nil {
		return Post{}, fmt.Errorf("front matter: %w", err)
	}
	if meta.Title == "" {
		return Post{}, errors.New("front matter: missing title")
	}
	if meta.Date.IsZero() {
		return Post{}, errors.New("front matter: missing date")
	}
	var html bytes.Buffer
	if err := markdown.Convert(body, &html); err != nil {
		return Post{}, fmt.Errorf("markdown: %w", err)
	}
	return Post{Title: meta.Title, Date: meta.Date, Body: templ.Raw(html.String())}, nil
}

func splitFrontMatter(src []byte) (front, body []byte, err error) {
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	rest, ok := bytes.CutPrefix(src, []byte("---\n"))
	if !ok {
		return nil, nil, errors.New("missing front matter")
	}
	// Keep the newline before the closing delimiter so that an empty block
	// is still found, and so the YAML keeps its final line ending.
	end := bytes.Index(append([]byte("\n"), rest...), []byte("\n---\n"))
	if end < 0 {
		return nil, nil, errors.New("unterminated front matter")
	}
	return rest[:end], rest[end+len("---\n"):], nil
}
