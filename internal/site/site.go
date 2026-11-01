// Package site holds the website's content: templ layouts, pages and notes.
package site

import (
	"embed"
	"time"

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
	notes, err := ssg.LoadNotes(noteFiles, "notes/*.md")
	if err != nil {
		return nil, err
	}
	more := len(notes) > homeNotes
	pages := []ssg.Page{
		{Path: "index.html", Content: Index(notes[:min(homeNotes, len(notes))], more)},
		{Path: "404.html", Content: NotFound()},
	}
	if more {
		pages = append(pages, ssg.Page{Path: "notes/index.html", Content: Notes(notes)})
	}
	for _, p := range notes {
		pages = append(pages, ssg.Page{
			Path:    "notes/" + p.Slug + "/index.html",
			Content: Note(p),
		})
	}
	return pages, nil
}

func isoDate(t time.Time) string {
	return t.Format(time.DateOnly)
}

func longDate(t time.Time) string {
	return t.Format("2 January 2006")
}
