// Package site holds the website's content: templ layouts, pages and notes.
package site

import (
	"embed"
	"io/fs"
	"strings"
	"time"

	"github.com/a-h/templ"
	"go.vhespanha.dev/www/internal/ssg"
)

// homeNotes is how many notes fit on the home page. The index page exists only
// when there are more.
const homeNotes = 5

// Assets holds the static files served under /assets/.
//
//go:embed assets
var Assets embed.FS

//go:embed notes/*.md
var noteFiles embed.FS

// Pages returns every page of the website.
func Pages() ([]ssg.Page, error) {
	return pages(noteFiles, "notes/*.md")
}

func pages(fsys fs.FS, pattern string) ([]ssg.Page, error) {
	notes, err := ssg.LoadNotes(fsys, pattern, components, portuguese{}.key())
	if err != nil {
		return nil, err
	}
	more := len(notes) > homeNotes

	var pages []ssg.Page
	add := func(p page, content func(page) templ.Component) {
		pages = append(pages, ssg.Page{Path: p.file(), Content: content(p)})
	}
	for _, l := range locales {
		// Every locale lists every note: one without a translation links to the
		// original, marked with its language.
		items := noteItems(l, notes)
		add(page{l: l, path: "/", translated: true}, func(p page) templ.Component {
			return Index(p, items[:min(homeNotes, len(items))], more)
		})
		add(page{l: l}, func(p page) templ.Component { return NotFound(p) })
		if more {
			add(page{l: l, path: "/notes/", translated: true}, func(p page) templ.Component {
				return Notes(p, items)
			})
		}
	}
	for _, n := range notes {
		path := "/notes/" + n.Slug + "/"
		for _, l := range locales {
			text, ok := noteIn(n, l)
			if !ok {
				continue
			}
			_, translated := noteIn(n, l.other())
			p := page{l: l, path: path, translated: translated, head: text.Head}
			add(p, func(p page) templ.Component { return Note(p, text) })
		}
	}
	return pages, nil
}

// noteIn returns n in the language of l, if it exists.
func noteIn(n ssg.Note, l locale) (ssg.Note, bool) {
	if l.key() == "" {
		return n, true
	}
	t, ok := n.Translations[l.key()]
	return t, ok
}

// A noteItem is an entry in a list of notes.
type noteItem struct {
	title string
	href  string
	date  time.Time
	// original is the locale of an untranslated note listed in another locale,
	// and nil otherwise.
	original locale
}

func noteItems(l locale, notes []ssg.Note) []noteItem {
	items := make([]noteItem, 0, len(notes))
	for _, n := range notes {
		path := "/notes/" + n.Slug + "/"
		if t, ok := noteIn(n, l); ok {
			items = append(items, noteItem{title: t.Title, href: localeURL(l, path), date: n.Date})
		} else {
			items = append(items, noteItem{title: n.Title, href: path, date: n.Date, original: english{}})
		}
	}
	return items
}

// A page identifies a page of the website in one language.
type page struct {
	l locale
	// path is the page's URL path without any language prefix, or empty for a
	// page with no URL of its own, like the 404 page.
	path string
	// translated reports whether the page also exists in the other language.
	translated bool
	// head is added to the page's head, such as the scripts of the
	// components a note uses.
	head []templ.Component
}

// file returns the page's output file.
func (p page) file() string {
	if p.path == "" {
		return strings.TrimPrefix(localeURL(p.l, "/404.html"), "/")
	}
	return strings.TrimPrefix(localeURL(p.l, p.path), "/") + "index.html"
}

func isoDate(t time.Time) string {
	return t.Format(time.DateOnly)
}
