package ssg

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"unicode/utf8"

	"github.com/a-h/templ"
)

func TestLoadNotes(t *testing.T) {
	fsys := fstest.MapFS{
		"notes/old.md":      {Data: []byte("---\ntitle: Old\ndate: 2025-01-02\n---\n# Heading\n\nNote[^1]\n\n[^1]: Text\n\n<aside>raw</aside>\n\n\"Quote\" -- it's...\n")},
		"notes/new.md":      {Data: []byte("---\r\ntitle: New\r\ndate: 2026-01-02\r\n---\r\nBody\r\n")},
		"notes/ignored.txt": {Data: []byte("not a note")},
	}
	notes, err := LoadNotes(fsys, "notes/*.md", nil)
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
	for _, want := range []string{"Heading</h1>", `class="footnotes"`, "<aside>raw</aside>", "&ldquo;Quote&rdquo; &ndash; it&rsquo;s&hellip;"} {
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
			_, err := LoadNotes(fsys, "*.md", nil)
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
	notes, err := LoadNotes(fsys, "*.md", nil, "pt")
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
			_, err := LoadNotes(fsys, "*.md", nil, "pt")
			if err == nil {
				t.Fatal("got nil error")
			}
			if !strings.Contains(err.Error(), ".md: ") {
				t.Errorf("error %q does not name the file", err)
			}
		})
	}
}

func TestLoadNotesInvalidSlug(t *testing.T) {
	src := []byte("---\ntitle: A\ndate: 2026-01-02\n---\n")
	for _, name := range []string{"c#-tips.md", "Upper.md", "a b.md", "Upper.pt.md"} {
		_, err := LoadNotes(fstest.MapFS{name: {Data: src}}, "*.md", nil, "pt")
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

	link := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		u, err := Asset(ctx, "/assets/site.css")
		if err != nil {
			return err
		}
		_, err = io.WriteString(w, u)
		return err
	})
	pages := []Page{
		{Path: "notes/a/index.html", Content: templ.Raw("<p>a</p>")},
		{Path: "link.txt", Content: link},
	}
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
	u, err := os.ReadFile(filepath.Join(out, "link.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if !regexp.MustCompile(`^/assets/site\.[0-9a-f]{10}\.css$`).Match(u) {
		t.Errorf("got asset URL %q, want a fingerprinted one", u)
	}
	if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(string(u)))); err != nil {
		t.Errorf("fingerprinted asset missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(out, "assets", "site.css")); !os.IsNotExist(err) {
		t.Errorf("unfingerprinted asset left behind: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(stale)); !os.IsNotExist(err) {
		t.Errorf("stale output not removed: %v", err)
	}
}

func TestBuildUnknownAsset(t *testing.T) {
	page := templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		_, err := Asset(ctx, "/assets/missing.js")
		return err
	})
	err := Build(context.Background(), t.TempDir(), []Page{{Path: "a.html", Content: page}}, fstest.MapFS{})
	if err == nil || !strings.Contains(err.Error(), "/assets/missing.js") {
		t.Errorf("got error %v, want one naming the missing asset", err)
	}
}

func TestAssetOutsideBuild(t *testing.T) {
	if got, err := Asset(context.Background(), "/assets/a.js"); got != "/assets/a.js" || err != nil {
		t.Errorf("got %q, %v; want the path unchanged", got, err)
	}
}

func TestBuildBundlesCSS(t *testing.T) {
	// A relative output directory, as the command line usually gives.
	t.Chdir(t.TempDir())
	out := "dist"
	assets := fstest.MapFS{
		"assets/site.css": {Data: []byte("@layer a, b;\n@import \"./base.css\";\n")},
		"assets/base.css": {Data: []byte(`
@media (prefers-reduced-motion: no-preference) {
  @view-transition {
    navigation: auto;
  }
}
@font-face {
  font-family: "F";
  src: url("fonts/f.woff2") format("woff2");
}
@layer b {
  p {
    color: red;
    background: url("data:image/gif;base64,R0lGODlhAQABAAAAACw=");
    & a {
      color: blue;
    }
  }
}
`)},
		"assets/fonts/f.woff2": {Data: []byte("font")},
	}
	if err := Build(context.Background(), out, nil, assets); err != nil {
		t.Fatal(err)
	}

	sheets, err := filepath.Glob(filepath.Join(out, "assets", "*.css"))
	if err != nil {
		t.Fatal(err)
	}
	if len(sheets) != 1 {
		t.Fatalf("got stylesheets %v, want only the bundle", sheets)
	}
	got, err := os.ReadFile(sheets[0])
	if err != nil {
		t.Fatal(err)
	}
	css := string(got)
	if !strings.HasPrefix(css, "@layer a,b;") {
		t.Errorf("layer order not first: %q", css)
	}
	for _, want := range []string{"@view-transition{navigation:auto}", "url(data:image/gif", "& a{"} {
		if !strings.Contains(css, want) {
			t.Errorf("got %q, want %q", css, want)
		}
	}
	font := regexp.MustCompile(`url\((/assets/fonts/f\.[0-9a-f]{10}\.woff2)\)`).FindStringSubmatch(css)
	if font == nil {
		t.Fatalf("font URL not fingerprinted: %q", css)
	}
	if strings.Contains(css, "@import") {
		t.Errorf("import not bundled: %q", css)
	}
	if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(font[1]))); err != nil {
		t.Errorf("font not copied: %v", err)
	}
}

func TestBuildUnknownCSSURL(t *testing.T) {
	assets := fstest.MapFS{"assets/site.css": {Data: []byte(`a{background:url("missing.png")}`)}}
	if err := Build(context.Background(), t.TempDir(), nil, assets); err == nil {
		t.Error("got nil error for a url() to a missing asset")
	}
}

func TestLoadNotesMetadata(t *testing.T) {
	long := strings.Repeat("word ", 40)
	fsys := fstest.MapFS{
		"set.md":     {Data: []byte("---\ntitle: A\ndate: 2026-01-02\ndescription: Given.\n---\nBody\n")},
		"derived.md": {Data: []byte("---\ntitle: B\ndate: 2026-01-03\n---\n\n# Heading\n\nIt's *the* first[^1]\nparagraph <b>x</b>.\n\nSecond.\n\n[^1]: Note.\n")},
		"long.md":    {Data: []byte("---\ntitle: C\ndate: 2026-01-04\n---\n" + long + "\n")},
		"table.md":   {Data: []byte("---\ntitle: D\ndate: 2026-01-05\n---\n| a | b |\n|---|---|\n| 1 | 2 |\n")},
	}
	notes, err := LoadNotes(fsys, "*.md", nil)
	if err != nil {
		t.Fatal(err)
	}
	bySlug := make(map[string]Note)
	for _, n := range notes {
		bySlug[n.Slug] = n
	}
	if got := bySlug["set"].Description; got != "Given." {
		t.Errorf("got description %q, want the front matter's", got)
	}
	if got, want := bySlug["derived"].Description, "It’s the first paragraph x."; got != want {
		t.Errorf("got description %q, want %q", got, want)
	}
	if got := bySlug["long"].Description; utf8.RuneCountInString(got) > excerptLen+1 || !strings.HasSuffix(got, "word…") {
		t.Errorf("got description %q, want one cut at a word", got)
	}
	if got := string(bySlug["set"].Source); got != "Body\n" {
		t.Errorf("got source %q, want the markdown without front matter", got)
	}
	var buf bytes.Buffer
	if err := bySlug["table"].Body.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	if got := buf.String(); strings.Count(got, `<th scope="col">`) != 2 {
		t.Errorf("header cells lack scope: %q", got)
	}
}

func TestLoadNotesComponents(t *testing.T) {
	type props struct {
		N int `yaml:"n"`
	}
	components := map[string]Component{
		"num": NewComponent(func(p props) templ.Component {
			return templ.Raw(fmt.Sprintf("<b>%d</b>", p.N))
		}, templ.Raw("<script></script>")),
		"bare": NewComponent(func(struct{}) templ.Component {
			return templ.Raw("<i>bare</i>")
		}, nil),
	}
	const front = "---\ntitle: A\ndate: 2026-01-02\n---\n"
	src := front +
		"```@num\nn: 1\n```\n\n" +
		"- item\n\n  ```@num\n  n: 2\n  ```\n\n" +
		"```@bare\n```\n\n" +
		"```go\nx := 1\n```\n"
	fsys := fstest.MapFS{"a.md": {Data: []byte(src)}}
	notes, err := LoadNotes(fsys, "*.md", components)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := notes[0].Body.Render(context.Background(), &buf); err != nil {
		t.Fatal(err)
	}
	wants := []string{
		"<b>1</b>",
		"<li>\n<p>item</p>\n<b>2</b>",
		"<i>bare</i>",
		`<code class="language-go">`,
	}
	for _, want := range wants {
		if got := buf.String(); !strings.Contains(got, want) {
			t.Errorf("got body %q, want %q", got, want)
		}
	}
	if len(notes[0].Head) != 1 {
		t.Errorf("got %d head components, want 1", len(notes[0].Head))
	}

	for name, body := range map[string]string{
		"unknown":    "```@nope\n```\n",
		"bad prop":   "```@num\nm: 1\n```\n",
		"bad yaml":   "```@num\nn: [\n```\n",
		"wrong type": "```@num\nn: x\n```\n",
	} {
		t.Run(name, func(t *testing.T) {
			fsys := fstest.MapFS{"a.md": {Data: []byte(front + body)}}
			if _, err := LoadNotes(fsys, "*.md", components); err == nil {
				t.Fatal("got nil error")
			}
		})
	}
}
