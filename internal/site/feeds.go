package site

import (
	"encoding/xml"
	"time"

	"github.com/a-h/templ"
)

// A listing is a page listed in the sitemap.
type listing struct {
	p       page
	lastmod time.Time
}

func sitemap(listings []listing) templ.Component {
	type link struct {
		Rel      string `xml:"rel,attr"`
		Hreflang string `xml:"hreflang,attr"`
		Href     string `xml:"href,attr"`
	}
	type url struct {
		Loc     string `xml:"loc"`
		Lastmod string `xml:"lastmod,omitempty"`
		Links   []link `xml:"xhtml:link"`
	}
	set := struct {
		XMLName xml.Name `xml:"urlset"`
		NS      string   `xml:"xmlns,attr"`
		XHTML   string   `xml:"xmlns:xhtml,attr"`
		URLs    []url    `xml:"url"`
	}{
		NS:    "http://www.sitemaps.org/schemas/sitemap/0.9",
		XHTML: "http://www.w3.org/1999/xhtml",
	}
	for _, s := range listings {
		u := url{Loc: s.p.canonical()}
		if !s.lastmod.IsZero() {
			u.Lastmod = isoDate(s.lastmod)
		}
		if s.p.translated {
			for _, l := range locales {
				u.Links = append(u.Links, link{"alternate", l.tag(), origin + localeURL(l, s.p.path)})
			}
			u.Links = append(u.Links, link{"alternate", "x-default", origin + s.p.path})
		}
		set.URLs = append(set.URLs, u)
	}
	return xmlFile(set)
}

// feed is the RSS feed of the notes listed in l, newest first.
func feed(l locale, items []noteItem) templ.Component {
	type guid struct {
		IsPermaLink bool   `xml:"isPermaLink,attr"`
		Value       string `xml:",chardata"`
	}
	type item struct {
		Title       string `xml:"title"`
		Link        string `xml:"link"`
		GUID        guid   `xml:"guid"`
		PubDate     string `xml:"pubDate"`
		Description string `xml:"description,omitempty"`
	}
	type atomLink struct {
		Href string `xml:"href,attr"`
		Rel  string `xml:"rel,attr"`
		Type string `xml:"type,attr"`
	}
	type channel struct {
		Title           string   `xml:"title"`
		Link            string   `xml:"link"`
		Description     string   `xml:"description"`
		Language        string   `xml:"language"`
		Self            atomLink `xml:"atom:link"`
		LastBuildDate   string   `xml:"lastBuildDate,omitempty"`
		UpdatePeriod    string   `xml:"sy:updatePeriod"`
		UpdateFrequency int      `xml:"sy:updateFrequency"`
		Items           []item   `xml:"item"`
	}
	self := origin + localeURL(l, "/feed.xml")
	c := channel{
		Title:           l.feedTitle(),
		Link:            origin + localeURL(l, "/"),
		Description:     l.feedDescription(),
		Language:        l.tag(),
		Self:            atomLink{self, "self", "application/rss+xml"},
		UpdatePeriod:    "weekly",
		UpdateFrequency: 1,
	}
	if len(items) > 0 {
		c.LastBuildDate = rfc822(items[0].date)
	}
	for _, n := range items {
		u := origin + n.href
		c.Items = append(c.Items, item{
			Title:       n.title,
			Link:        u,
			GUID:        guid{true, u},
			PubDate:     rfc822(n.date),
			Description: n.description,
		})
	}
	return xmlFile(struct {
		XMLName xml.Name `xml:"rss"`
		Version string   `xml:"version,attr"`
		Atom    string   `xml:"xmlns:atom,attr"`
		Sy      string   `xml:"xmlns:sy,attr"`
		Channel channel  `xml:"channel"`
	}{
		Version: "2.0",
		Atom:    "http://www.w3.org/2005/Atom",
		Sy:      "http://purl.org/rss/1.0/modules/syndication/",
		Channel: c,
	})
}

// robotsTxt points crawlers at the sitemap.
func robotsTxt() templ.Component {
	return textFile("robots.txt.tmpl", nil)
}
