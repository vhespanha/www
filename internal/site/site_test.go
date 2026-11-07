package site

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"go.vhespanha.dev/www/internal/ssg"
)

// render builds the site from a translated and an untranslated note, and
// returns its pages by output file.
func render(t *testing.T) map[string]string {
	t.Helper()
	fsys := fstest.MapFS{
		"both.md":    {Data: []byte("---\ntitle: Both\ndate: 2026-02-01\ndescription: Both <of> them.\n---\nBody\n")},
		"both.pt.md": {Data: []byte("---\ntitle: Ambos\n---\nCorpo\n")},
		"solo.md":    {Data: []byte("---\ntitle: Solo\ndate: 2026-01-15\n---\nBody\n\n```@counter\nstart: 1\n```\n")},
	}
	pages, err := pages(fsys, "*.md")
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]string, len(pages))
	for _, p := range pages {
		var buf bytes.Buffer
		if err := p.Content.Render(context.Background(), &buf); err != nil {
			t.Fatalf("render %s: %v", p.Path, err)
		}
		if _, dup := out[p.Path]; dup {
			t.Errorf("two pages write %s", p.Path)
		}
		out[p.Path] = buf.String()
	}
	return out
}

func TestPagesExistInEachLanguage(t *testing.T) {
	got := render(t)
	for file, lang := range map[string]string{
		"index.html":               "en",
		"404.html":                 "en",
		"notes/both/index.html":    "en",
		"notes/solo/index.html":    "en",
		"pt/index.html":            "pt-BR",
		"pt/404.html":              "pt-BR",
		"pt/notes/both/index.html": "pt-BR",
	} {
		html, ok := got[file]
		if !ok {
			t.Errorf("missing page %s", file)
			continue
		}
		if want := `<html lang="` + lang + `">`; !strings.Contains(html, want) {
			t.Errorf("%s: missing %s", file, want)
		}
	}
	if _, ok := got["pt/notes/solo/index.html"]; ok {
		t.Error("an untranslated note must not get a page under /pt/")
	}
}

// Search engines ignore alternates that do not point back at each other, and
// the language switch must never lead to a 404.
func TestAlternatesAreReciprocal(t *testing.T) {
	got := render(t)
	link := regexp.MustCompile(`<link rel="alternate" hreflang="([^"]+)" href="` + regexp.QuoteMeta(origin) + `([^"]*)"`)
	for file, html := range got {
		for _, m := range link.FindAllStringSubmatch(html, -1) {
			target := strings.TrimPrefix(m[2], "/") + "index.html"
			other, ok := got[target]
			if !ok {
				t.Errorf("%s: alternate %s has no page", file, m[2])
				continue
			}
			self := origin + "/" + strings.TrimSuffix(file, "index.html")
			if !strings.Contains(other, `href="`+self+`"`) {
				t.Errorf("%s does not link back to %s", target, self)
			}
		}
	}
}

func TestUntranslatedPagesOfferNoSwitch(t *testing.T) {
	got := render(t)
	for _, file := range []string{"notes/solo/index.html", "404.html", "pt/404.html"} {
		for _, want := range []string{"data-other", "data-lang-choice", "lang.js"} {
			if strings.Contains(got[file], want) {
				t.Errorf("%s: unexpected %s", file, want)
			}
		}
	}
	if strings.Contains(got["404.html"], "canonical") {
		t.Error("404 page must not declare a canonical URL")
	}
}

func TestPortugueseHomeListsEveryNote(t *testing.T) {
	home := render(t)["pt/index.html"]
	for _, want := range []string{
		`href="/pt/notes/both/"`,
		`<a href="/notes/solo/" hreflang="en" lang="en">Solo</a>`,
		"em inglês",
	} {
		if !strings.Contains(home, want) {
			t.Errorf("missing %s in:\n%s", want, home)
		}
	}
}

func TestPortugueseLongDate(t *testing.T) {
	for _, tt := range []struct {
		date time.Time
		want string
	}{
		{time.Date(2026, time.November, 1, 0, 0, 0, 0, time.UTC), "1º de novembro de 2026"},
		{time.Date(2026, time.March, 15, 0, 0, 0, 0, time.UTC), "15 de março de 2026"},
	} {
		if got := (portuguese{}).longDate(tt.date); got != tt.want {
			t.Errorf("got %q, want %q", got, tt.want)
		}
	}
}

// htmlPages returns the HTML pages of the site by output file.
func htmlPages(t *testing.T) map[string]string {
	t.Helper()
	out := make(map[string]string)
	for file, content := range render(t) {
		if strings.HasSuffix(file, ".html") {
			out[file] = content
		}
	}
	return out
}

func TestEveryPageHasTheEssentials(t *testing.T) {
	for file, html := range htmlPages(t) {
		for _, want := range []string{
			"<!doctype html>",
			`<meta charset="utf-8">`,
			`<meta name="viewport" content="width=device-width, initial-scale=1">`,
			`<meta name="color-scheme" content="light dark">`,
			`<meta name="theme-color"`,
			`<link rel="icon" href="/favicon.ico"`,
			`<a class="skip-link" href="#main">`,
			`<main id="main"`,
			`<footer class="site-footer">`,
			`/privacy/"`,
		} {
			if !strings.Contains(strings.ToLower(html), strings.ToLower(want)) {
				t.Errorf("%s: missing %s", file, want)
			}
		}
		if n := strings.Count(html, "<h1"); n != 1 {
			t.Errorf("%s: %d <h1> elements, want 1", file, n)
		}
		if n := strings.Count(html, "<title>"); n != 1 {
			t.Errorf("%s: %d <title> elements, want 1", file, n)
		}
	}
}

func TestIndexablePagesDescribeThemselves(t *testing.T) {
	for file, html := range htmlPages(t) {
		if strings.HasSuffix(file, "404.html") {
			if !strings.Contains(html, `<meta name="robots" content="noindex">`) {
				t.Errorf("%s: missing noindex", file)
			}
			continue
		}
		for _, want := range []string{
			`<meta name="description" content="`,
			`<link rel="canonical"`,
			`<meta property="og:title"`,
			`<meta property="og:image" content="` + origin + `/assets/og.png"`,
			`<meta property="og:url"`,
			`<meta property="og:type"`,
			`<link rel="alternate" type="application/rss+xml"`,
		} {
			if !strings.Contains(html, want) {
				t.Errorf("%s: missing %s", file, want)
			}
		}
	}
}

// Notes describe themselves as articles and the home page its author. No
// other page has structured data.
func TestStructuredData(t *testing.T) {
	ld := regexp.MustCompile(`(?s)<script type="application/ld\+json">(.*?)</script>`)
	want := map[string]string{
		"index.html":               "Person",
		"pt/index.html":            "Person",
		"notes/both/index.html":    "BlogPosting",
		"pt/notes/both/index.html": "BlogPosting",
		"notes/solo/index.html":    "BlogPosting",
	}
	for file, html := range htmlPages(t) {
		m := ld.FindAllStringSubmatch(html, -1)
		if want[file] == "" {
			if m != nil {
				t.Errorf("%s: unexpected structured data", file)
			}
			continue
		}
		if len(m) != 1 {
			t.Errorf("%s: %d structured data scripts, want 1", file, len(m))
			continue
		}
		var data struct {
			Type string `json:"@type"`
		}
		if err := json.Unmarshal([]byte(m[0][1]), &data); err != nil || data.Type != want[file] {
			t.Errorf("%s: bad structured data %s: %v", file, m[0][1], err)
		}
	}
}

func TestNotesAreArticles(t *testing.T) {
	got := render(t)
	en, pt := got["notes/both/index.html"], got["pt/notes/both/index.html"]
	for _, want := range []string{
		`<meta name="description" content="Both &lt;of&gt; them.">`,
		`<meta property="og:type" content="article">`,
		`"datePublished":"2026-02-01"`,
		`"image":"` + origin + `/assets/og.png"`,
		`<link rel="alternate" type="text/markdown" href="/notes/both.md">`,
		`<nav class="kicker" aria-label="Breadcrumb">`,
		`<article class="prose">`,
	} {
		if !strings.Contains(en, want) {
			t.Errorf("missing %s in:\n%s", want, en)
		}
	}
	if want := `"inLanguage":"pt-BR"`; !strings.Contains(pt, want) {
		t.Errorf("missing %s", want)
	}
	md := got["notes/both.md"]
	if !strings.HasPrefix(md, "---\ntitle: Both\nurl: "+origin+"/notes/both/\n") || !strings.HasSuffix(md, "# Both\n\nBody\n") {
		t.Errorf("bad markdown source:\n%s", md)
	}
	if _, ok := got["pt/notes/both.md"]; !ok {
		t.Error("missing markdown source of the translation")
	}
}

func TestFeedsAndSitemapAreWellFormed(t *testing.T) {
	got := render(t)
	for _, file := range []string{"sitemap.xml", "feed.xml", "pt/feed.xml"} {
		dec := xml.NewDecoder(strings.NewReader(got[file]))
		for {
			_, err := dec.Token()
			if err == io.EOF {
				break
			}
			if err != nil {
				t.Errorf("%s: %v", file, err)
				break
			}
		}
	}
	sitemap := got["sitemap.xml"]
	for _, want := range []string{
		"<loc>" + origin + "/</loc>",
		"<loc>" + origin + "/pt/notes/both/</loc>",
		"<loc>" + origin + "/privacy/</loc>",
		"<lastmod>2026-02-01</lastmod>",
	} {
		if !strings.Contains(sitemap, want) {
			t.Errorf("sitemap: missing %s", want)
		}
	}
	if strings.Contains(sitemap, "404") {
		t.Error("sitemap lists the 404 page")
	}
	feed := got["feed.xml"]
	for _, want := range []string{
		`<atom:link href="` + origin + `/feed.xml" rel="self" type="application/rss+xml"></atom:link>`,
		`<guid isPermaLink="true">` + origin + `/notes/both/</guid>`,
		"<pubDate>Sun, 01 Feb 2026 00:00:00 +0000</pubDate>",
	} {
		if !strings.Contains(feed, want) {
			t.Errorf("feed: missing %s in\n%s", want, feed)
		}
	}
}

func TestTextFiles(t *testing.T) {
	got := render(t)
	if !strings.Contains(got["robots.txt"], "Sitemap: "+origin+"/sitemap.xml") {
		t.Error("robots.txt does not point at the sitemap")
	}
	if !strings.Contains(got["llms.txt"], "("+origin+"/pt/notes/both.md)") {
		t.Errorf("llms.txt does not list the translation:\n%s", got["llms.txt"])
	}
	if !strings.Contains(got["_redirects"], "/pt/notes/both /pt/notes/both/ 301\n") {
		t.Errorf("_redirects:\n%s", got["_redirects"])
	}
}

// Every local link and resource of a built page must exist, under the
// fingerprinted names the build gives assets.
func TestBuiltLinksResolve(t *testing.T) {
	fsys := fstest.MapFS{
		"a.md":    {Data: []byte("---\ntitle: A\ndate: 2026-02-01\n---\nBody\n\n```@counter\nstart: 1\n```\n")},
		"a.pt.md": {Data: []byte("---\ntitle: A\n---\nCorpo\n")},
	}
	pages, err := pages(fsys, "*.md")
	if err != nil {
		t.Fatal(err)
	}
	out := t.TempDir()
	if err := ssg.Build(context.Background(), out, pages, Assets); err != nil {
		t.Fatal(err)
	}
	ref := regexp.MustCompile(`(?:href|src)="(/[^"#]*)`)
	err = filepath.WalkDir(out, func(name string, d fs.DirEntry, err error) error {
		if err != nil || filepath.Ext(name) != ".html" {
			return err
		}
		html, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		for _, m := range ref.FindAllSubmatch(html, -1) {
			target := filepath.Join(out, filepath.FromSlash(string(m[1])))
			if strings.HasSuffix(string(m[1]), "/") {
				target = filepath.Join(target, "index.html")
			}
			if _, err := os.Stat(target); err != nil {
				t.Errorf("%s: broken reference %s", name, m[1])
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
