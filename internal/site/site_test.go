package site

import (
	"bytes"
	"context"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

// render builds the site from a translated and an untranslated note, and
// returns its pages by output file.
func render(t *testing.T) map[string]string {
	t.Helper()
	fsys := fstest.MapFS{
		"both.md":    {Data: []byte("---\ntitle: Both\ndate: 2026-02-01\n---\nBody\n")},
		"both.pt.md": {Data: []byte("---\ntitle: Ambos\n---\nCorpo\n")},
		"solo.md":    {Data: []byte("---\ntitle: Solo\ndate: 2026-01-15\n---\nBody\n")},
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
