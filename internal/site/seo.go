package site

import (
	"context"
	"time"

	"go.vhespanha.dev/www/internal/ssg"
)

// siteName names the website and its author, who are the same.
const siteName = "Vinicius Hespanha"

// The author's contact details.
const (
	emailAddress = "mail@vhespanha.dev"
	githubURL    = "https://github.com/vhespanha"
)

// ogImage is the asset shown when a page is shared, and its size in pixels.
const (
	ogImage       = "/assets/og.png"
	ogImageWidth  = "1200"
	ogImageHeight = "630"
)

// meta describes a page to browsers, search engines and link previews.
type meta struct {
	// heading is the page's own name, as in its <h1> and link previews.
	heading string
	// title is the document title. If empty, it is heading followed by the
	// site's name.
	title       string
	description string
	// published is the publication date of an article, and zero for any
	// other page.
	published time.Time
	// markdown is the URL path of the page's markdown source, if any.
	markdown string
	// crumbs are the pages above this one, from the home page down.
	crumbs []crumb
}

// A crumb is one step in a page's breadcrumb trail.
type crumb struct {
	label, href string
	// brand marks a label that is a name, which translation must leave alone.
	brand bool
}

func (m meta) documentTitle() string {
	if m.title != "" {
		return m.title
	}
	return m.heading + " · " + siteName
}

// asset returns the URL path of the asset at the URL path p. See [ssg.Asset].
func asset(ctx context.Context, p string) (string, error) {
	return ssg.Asset(ctx, p)
}

// absAsset returns the absolute URL of the asset at the URL path p.
func absAsset(ctx context.Context, p string) (string, error) {
	u, err := ssg.Asset(ctx, p)
	return origin + u, err
}

// canonical returns the absolute URL of p.
func (p page) canonical() string {
	return origin + localeURL(p.l, p.path)
}

// homeCrumb is the first step of every breadcrumb trail in l.
func homeCrumb(l locale) crumb {
	return crumb{label: "vhespanha", href: localeURL(l, "/"), brand: true}
}

// structuredData returns the schema.org entity describing p as JSON-LD: an
// article for a note and the author for the home page. It is nil for any other
// page.
func structuredData(ctx context.Context, p page, m meta) (map[string]any, error) {
	switch {
	case !m.published.IsZero():
		image, err := absAsset(ctx, ogImage)
		if err != nil {
			return nil, err
		}
		return map[string]any{
			"@context":      "https://schema.org",
			"@type":         "BlogPosting",
			"headline":      m.heading,
			"description":   m.description,
			"image":         image,
			"datePublished": isoDate(m.published),
			"inLanguage":    p.l.tag(),
			"url":           p.canonical(),
			"author":        map[string]any{"@type": "Person", "name": siteName, "url": origin + "/"},
		}, nil
	case p.path == "/":
		return map[string]any{
			"@context": "https://schema.org",
			"@type":    "Person",
			"name":     siteName,
			"url":      origin + "/",
			"jobTitle": p.l.jobTitle(),
			"email":    "mailto:" + emailAddress,
			"sameAs":   []string{githubURL},
			"homeLocation": map[string]any{
				"@type": "Place",
				"name":  "São Paulo, Brazil",
			},
		}, nil
	}
	return nil, nil
}

// rfc822 formats t as RSS dates require.
func rfc822(t time.Time) string {
	return t.UTC().Format(time.RFC1123Z)
}
