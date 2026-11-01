// Package site holds the website's content: templ layouts, pages and posts.
package site

import (
	"embed"

	"go.vhespanha.dev/www/internal/ssg"
)

//go:embed posts/*.md
var postFiles embed.FS

// Pages returns every page of the website.
func Pages() ([]ssg.Page, error) {
	posts, err := ssg.LoadPosts(postFiles, "posts/*.md")
	if err != nil {
		return nil, err
	}
	pages := []ssg.Page{
		{Path: "index.html", Content: Index()},
		{Path: "posts/index.html", Content: Posts(posts)},
	}
	for _, p := range posts {
		pages = append(pages, ssg.Page{
			Path:    "posts/" + p.Slug + "/index.html",
			Content: Post(p),
		})
	}
	return pages, nil
}
