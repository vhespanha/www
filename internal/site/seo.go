package site

import (
	"context"
	"maps"
	"slices"
	"time"

	"go.vhespanha.dev/www/internal/ssg"
)

// siteName names the website and its author, who are the same.
const siteName = "Vinicius Hespanha"

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
	// data is the page's own schema.org entity, merged into the page's
	// structured data. It may be nil.
	data map[string]any
}

// A crumb is one step in a page's breadcrumb trail.
type crumb struct {
	// label is shown on the page; name is the step's name in structured data.
	label, name string
	href        string
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
	return crumb{label: "vhespanha", name: siteName, href: localeURL(l, "/"), brand: true}
}

// IDs of the entities that every page's structured data refers to.
var (
	personID  = origin + "/#person"
	websiteID = origin + "/#website"
)

func person(l locale) map[string]any {
	return map[string]any{
		"@type":    "Person",
		"@id":      personID,
		"name":     siteName,
		"url":      origin + "/",
		"jobTitle": l.jobTitle(),
		"email":    "mailto:mail@vhespanha.dev",
		"sameAs":   []string{"https://github.com/vhespanha"},
		"homeLocation": map[string]any{
			"@type": "Place",
			"name":  "São Paulo, Brazil",
		},
	}
}

func website() map[string]any {
	tags := make([]string, len(locales))
	for i, l := range locales {
		tags[i] = l.tag()
	}
	return map[string]any{
		"@type":      "WebSite",
		"@id":        websiteID,
		"url":        origin + "/",
		"name":       siteName,
		"inLanguage": tags,
		"publisher":  map[string]any{"@id": personID},
	}
}

// structuredData returns the schema.org graph describing p as JSON-LD, or nil
// for a page with no URL of its own.
func structuredData(ctx context.Context, p page, m meta) (map[string]any, error) {
	if p.path == "" {
		return nil, nil
	}
	image, err := absAsset(ctx, ogImage)
	if err != nil {
		return nil, err
	}
	url := p.canonical()
	entity := map[string]any{
		"@type":       "WebPage",
		"@id":         url,
		"url":         url,
		"name":        m.heading,
		"description": m.description,
		"inLanguage":  p.l.tag(),
		"isPartOf":    map[string]any{"@id": websiteID},
		"image":       image,
	}
	maps.Copy(entity, m.data)
	graph := []any{website(), person(p.l), entity}
	if len(m.crumbs) > 0 {
		items := make([]any, 0, len(m.crumbs)+1)
		for i, c := range slices.Concat(m.crumbs, []crumb{{name: m.heading, href: localeURL(p.l, p.path)}}) {
			items = append(items, map[string]any{
				"@type":    "ListItem",
				"position": i + 1,
				"name":     c.name,
				"item":     origin + c.href,
			})
		}
		entity["breadcrumb"] = map[string]any{"@id": url + "#breadcrumb"}
		graph = append(graph, map[string]any{
			"@type":           "BreadcrumbList",
			"@id":             url + "#breadcrumb",
			"itemListElement": items,
		})
	}
	return map[string]any{"@context": "https://schema.org", "@graph": graph}, nil
}

// article returns the schema.org entity of a note published on date, whose
// pages in other languages are at the URL paths in translations.
func article(p page, n ssg.Note, translations []string) map[string]any {
	data := map[string]any{
		"@type":            "BlogPosting",
		"headline":         n.Title,
		"datePublished":    isoDate(n.Date),
		"mainEntityOfPage": p.canonical(),
		"author":           map[string]any{"@id": personID, "name": siteName, "url": origin + "/"},
	}
	var others []any
	for _, t := range translations {
		others = append(others, map[string]any{"@id": origin + t})
	}
	switch {
	case others == nil:
	case p.l.key() == "":
		data["workTranslation"] = others
	default:
		data["translationOfWork"] = others[0]
	}
	return data
}

// rfc822 formats t as RSS dates require.
func rfc822(t time.Time) string {
	return t.UTC().Format(time.RFC1123Z)
}
