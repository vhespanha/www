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

// privacyUpdated is when the privacy policy last changed.
var privacyUpdated = time.Date(2026, time.November, 3, 0, 0, 0, 0, time.UTC)

// Pages returns every page of the website, and the files beside them, as
// built at the time now.
func Pages(now time.Time) ([]ssg.Page, error) {
	return pages(noteFiles, "notes/*.md", now)
}

func pages(fsys fs.FS, pattern string, now time.Time) ([]ssg.Page, error) {
	notes, err := ssg.LoadNotes(fsys, pattern, components, portuguese{}.key())
	if err != nil {
		return nil, err
	}
	more := len(notes) > homeNotes
	var latest time.Time
	if len(notes) > 0 {
		latest = notes[0].Date
	}

	var (
		pages    []ssg.Page
		listings []listing
		sources  []markdownNote
	)
	// add adds the page p, listing it in the sitemap unless it has no URL.
	add := func(p page, lastmod time.Time, content templ.Component) {
		pages = append(pages, ssg.Page{Path: p.file(), Content: content})
		if p.path != "" {
			listings = append(listings, listing{p, lastmod})
		}
	}
	addFile := func(path string, content templ.Component) {
		pages = append(pages, ssg.Page{Path: strings.TrimPrefix(path, "/"), Content: content})
	}
	for _, l := range locales {
		// Every locale lists every note: one without a translation links to the
		// original, marked with its language.
		items := noteItems(l, notes)
		list := notesURL(l, more)
		home := page{l: l, path: "/", translated: true, notes: list}
		add(home, latest, Index(home, items[:min(homeNotes, len(items))], more))
		missing := page{l: l, notes: list}
		add(missing, time.Time{}, NotFound(missing))
		privacy := page{l: l, path: "/privacy/", translated: true, notes: list}
		add(privacy, privacyUpdated, Privacy(privacy))
		if more {
			index := page{l: l, path: "/notes/", translated: true, notes: list}
			add(index, latest, Notes(index, items))
		}
		addFile(localeURL(l, "/feed.xml"), feed(l, items))
		addFile(localeURL(l, "/manifest.webmanifest"), webManifest(l))
	}
	for _, n := range notes {
		path := "/notes/" + n.Slug + "/"
		for _, l := range locales {
			text, ok := noteIn(n, l)
			if !ok {
				continue
			}
			var translations []string
			for _, other := range locales {
				if _, ok := noteIn(n, other); ok && other != l {
					translations = append(translations, localeURL(other, path))
				}
			}
			p := page{
				l:          l,
				path:       path,
				translated: len(translations) > 0,
				head:       text.Head,
				notes:      notesURL(l, more),
			}
			add(p, n.Date, Note(p, text, translations))
			src := markdownNote{p, text}
			sources = append(sources, src)
			addFile(markdownURL(p), src.file())
		}
	}
	addFile("/sitemap.xml", sitemap(listings))
	addFile("/robots.txt", robotsTxt())
	addFile("/llms.txt", llmsTxt(sources))
	addFile("/llms-full.txt", llmsFullTxt(sources))
	addFile("/.well-known/security.txt", securityTxt(now))
	addFile("/.well-known/api-catalog", apiCatalog())
	addFile("/favicon.ico", constant(faviconICO))
	addFile("/_headers", headers())
	addFile("/_redirects", redirects(listings))
	return pages, nil
}

// notesURL returns the URL path of the list of every note in l: the notes
// page, if there are more than the home page shows, or else the home page's
// list.
func notesURL(l locale, more bool) string {
	if more {
		return localeURL(l, "/notes/")
	}
	return localeURL(l, "/") + "#notes"
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
	title       string
	description string
	href        string
	date        time.Time
	// original is the locale of an untranslated note listed in another locale,
	// and nil otherwise.
	original locale
}

func noteItems(l locale, notes []ssg.Note) []noteItem {
	items := make([]noteItem, 0, len(notes))
	for _, n := range notes {
		path := "/notes/" + n.Slug + "/"
		if t, ok := noteIn(n, l); ok {
			items = append(items, noteItem{title: t.Title, description: t.Description, href: localeURL(l, path), date: n.Date})
		} else {
			items = append(items, noteItem{title: n.Title, description: n.Description, href: path, date: n.Date, original: english{}})
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
	// notes is the URL path of the list of every note in the page's
	// language, for the breadcrumb trail.
	notes string
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
