// Package ssg implements the static site generator.
package ssg

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer"
	"github.com/yuin/goldmark/renderer/html"
	"github.com/yuin/goldmark/text"
	"github.com/yuin/goldmark/util"
	"go.yaml.in/yaml/v3"
)

// A Page is a component rendered to a file in the output directory.
type Page struct {
	// Path is the slash-separated output file, relative to the output
	// directory. For example, "notes/hello/index.html".
	Path    string
	Content templ.Component
}

// Build replaces the contents of the directory out with the files of assets,
// fingerprinted as described in [Asset] and with their stylesheets bundled
// into one, and the rendered pages.
func Build(ctx context.Context, out string, pages []Page, assets fs.FS) error {
	if err := checkOut(out); err != nil {
		return err
	}
	if err := clearDir(out); err != nil {
		return err
	}
	if err := os.CopyFS(out, assets); err != nil {
		return err
	}
	urls, err := fingerprint(out)
	if err != nil {
		return err
	}
	ctx = context.WithValue(ctx, assetsKey{}, urls)
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

// checkOut refuses an output directory that contains the working directory or
// the home directory, since Build deletes everything in it.
func checkOut(out string) error {
	abs, err := filepath.Abs(out)
	if err != nil {
		return err
	}
	wd, err := os.Getwd()
	if err != nil {
		return err
	}
	protected := []string{wd}
	if home, err := os.UserHomeDir(); err == nil {
		protected = append(protected, home)
	}
	for _, p := range protected {
		rel, err := filepath.Rel(abs, p)
		if err != nil {
			return err
		}
		if filepath.IsLocal(rel) {
			return fmt.Errorf("refusing to clear output directory %s: it contains %s", abs, p)
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

// A Note is a note parsed from a markdown file.
type Note struct {
	// Slug is the name of the source file without its extension.
	Slug  string
	Title string
	// Description summarises the note for search results and link previews.
	// It is the front matter's description, or else the start of the note's
	// first paragraph.
	Description string
	Date        time.Time
	// Minutes is the estimated reading time, at least 1.
	Minutes int
	// Body is the note's markdown rendered to HTML.
	Body templ.Component
	// Source is the note's markdown, without its front matter.
	Source []byte
	// Head holds the head content of the components the note uses, once
	// each, in order of first use.
	Head []templ.Component
	// Translations holds the note's translations by language. It is nil on a
	// translation.
	Translations map[string]Note
}

// LoadNotes parses the markdown files in fsys matching the glob pattern and
// returns the originals newest first, each with its translations attached.
// Notes embed components by name, as described in [Component].
//
// Each original must start with a YAML front matter block setting title and
// date, and optionally description:
//
//	---
//	title: Hello
//	date: 2026-11-01
//	description: A first note.
//	---
//
// A file named hello.pt.md is the translation of hello.md into the language
// pt, which must be one of langs. It must set its own title and takes its date
// from the original; it may repeat that date but not change it. A translation
// without an original or in an unknown language is an error, so that a typo in
// a file name cannot silently drop content.
func LoadNotes(
	fsys fs.FS,
	pattern string,
	components map[string]Component,
	langs ...string,
) ([]Note, error) {
	names, err := fs.Glob(fsys, pattern)
	if err != nil {
		return nil, err
	}
	type translation struct {
		name, lang string
		note       Note
	}
	md := newMarkdown(components)
	var (
		notes        = make([]Note, 0, len(names))
		translations []translation
	)
	for _, name := range names {
		src, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		note, err := parseNote(md, src)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		slug, lang, translated := strings.Cut(strings.TrimSuffix(path.Base(name), path.Ext(name)), ".")
		if !validSlug.MatchString(slug) {
			return nil, fmt.Errorf("%s: slug must match %s", name, validSlug)
		}
		note.Slug = slug
		if translated {
			if !slices.Contains(langs, lang) {
				return nil, fmt.Errorf("%s: unknown language %q", name, lang)
			}
			translations = append(translations, translation{name, lang, note})
			continue
		}
		if note.Date.IsZero() {
			return nil, fmt.Errorf("%s: front matter: missing date", name)
		}
		notes = append(notes, note)
	}
	for _, t := range translations {
		i := slices.IndexFunc(notes, func(n Note) bool { return n.Slug == t.note.Slug })
		if i < 0 {
			return nil, fmt.Errorf("%s: no original %s.md", t.name, t.note.Slug)
		}
		if !t.note.Date.IsZero() && !t.note.Date.Equal(notes[i].Date) {
			return nil, fmt.Errorf("%s: date differs from the original", t.name)
		}
		t.note.Date = notes[i].Date
		if notes[i].Translations == nil {
			notes[i].Translations = make(map[string]Note)
		}
		notes[i].Translations[t.lang] = t.note
	}
	slices.SortFunc(notes, func(a, b Note) int {
		return cmp.Or(b.Date.Compare(a.Date), cmp.Compare(a.Slug, b.Slug))
	})
	return notes, nil
}

const wordsPerMinute = 230

// validSlug matches the slugs that are safe to use unescaped in a URL.
var validSlug = regexp.MustCompile(`^[a-z0-9-]+$`)

func newMarkdown(components map[string]Component) goldmark.Markdown {
	return goldmark.New(
		goldmark.WithExtensions(extension.GFM, extension.Footnote, extension.Typographer),
		goldmark.WithParserOptions(parser.WithASTTransformers(
			util.Prioritized(componentTransformer(components), 100),
			util.Prioritized(headerScopes{}, 200),
		)),
		goldmark.WithRendererOptions(
			html.WithUnsafe(),
			renderer.WithNodeRenderers(util.Prioritized(componentRenderer{}, 100)),
		),
	)
}

func parseNote(md goldmark.Markdown, src []byte) (Note, error) {
	front, body, err := splitFrontMatter(src)
	if err != nil {
		return Note{}, err
	}
	var meta struct {
		Title       string    `yaml:"title"`
		Date        time.Time `yaml:"date"`
		Description string    `yaml:"description"`
	}
	dec := yaml.NewDecoder(bytes.NewReader(front))
	if err := dec.Decode(&meta); err != nil && !errors.Is(err, io.EOF) {
		return Note{}, fmt.Errorf("front matter: %w", err)
	}
	// A second document means a closing delimiter was missed, such as one with
	// trailing whitespace, and body text was consumed as front matter.
	if err := dec.Decode(new(any)); !errors.Is(err, io.EOF) {
		return Note{}, errors.New("front matter: more than one YAML document")
	}
	if meta.Title == "" {
		return Note{}, errors.New("front matter: missing title")
	}
	var rendered bytes.Buffer
	uses := new(componentUses)
	pc := parser.NewContext()
	pc.Set(usesKey, uses)
	doc := md.Parser().Parse(text.NewReader(body), parser.WithContext(pc))
	if uses.err != nil {
		return Note{}, uses.err
	}
	if err := md.Renderer().Render(&rendered, body, doc); err != nil {
		return Note{}, fmt.Errorf("markdown: %w", err)
	}
	description := meta.Description
	if description == "" {
		description = excerpt(doc, body)
	}
	return Note{
		Title:       meta.Title,
		Description: description,
		Date:        meta.Date,
		Minutes:     max(1, (len(bytes.Fields(body))+wordsPerMinute-1)/wordsPerMinute),
		Body:        templ.Raw(rendered.String()),
		Source:      bytes.TrimLeft(body, "\n"),
		Head:        uses.heads,
	}, nil
}

func splitFrontMatter(src []byte) (front, body []byte, err error) {
	src = bytes.ReplaceAll(src, []byte("\r\n"), []byte("\n"))
	if !bytes.HasPrefix(src, []byte("---\n")) {
		return nil, nil, errors.New("missing front matter")
	}
	// Cut after the opening dashes, so that an empty block still has the
	// newline that the closing delimiter is matched with.
	front, body, ok := bytes.Cut(src[len("---"):], []byte("\n---\n"))
	if !ok {
		return nil, nil, errors.New("unterminated front matter")
	}
	return front, body, nil
}
