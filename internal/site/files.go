package site

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"slices"
	"strings"
	"time"

	"github.com/a-h/templ"
	"go.vhespanha.dev/www/internal/ssg"
	"go.yaml.in/yaml/v3"
)

// This file renders the files beside the pages: those for crawlers, feed
// readers and agents, and the configuration Cloudflare reads from the output.

//go:embed static/favicon.ico
var faviconICO []byte

// file returns a component that writes what render returns.
func file(render func(ctx context.Context) ([]byte, error)) templ.Component {
	return templ.ComponentFunc(func(ctx context.Context, w io.Writer) error {
		b, err := render(ctx)
		if err != nil {
			return err
		}
		_, err = w.Write(b)
		return err
	})
}

// constant returns a component that writes b.
func constant(b []byte) templ.Component {
	return file(func(context.Context) ([]byte, error) { return b, nil })
}

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

func xmlFile(v any) templ.Component {
	return file(func(context.Context) ([]byte, error) {
		b, err := xml.MarshalIndent(v, "", "  ")
		if err != nil {
			return nil, err
		}
		return append([]byte(xml.Header), append(b, '\n')...), nil
	})
}

func jsonFile(v any) templ.Component {
	return file(func(context.Context) ([]byte, error) {
		b, err := json.MarshalIndent(v, "", "  ")
		return append(b, '\n'), err
	})
}

// A markdownNote is a note in one language, as published in markdown.
type markdownNote struct {
	p    page
	note ssg.Note
}

// markdownURL returns the URL path of the markdown source of the note page p:
// its URL with ".md" in place of the trailing slash.
func markdownURL(p page) string {
	return strings.TrimSuffix(localeURL(p.l, p.path), "/") + ".md"
}

// markdown renders a note as markdown, with front matter describing it.
func (n markdownNote) markdown() ([]byte, error) {
	front, err := yaml.Marshal(struct {
		Title       string `yaml:"title"`
		URL         string `yaml:"url"`
		Date        string `yaml:"date"`
		Language    string `yaml:"language"`
		Description string `yaml:"description,omitempty"`
		Author      string `yaml:"author"`
	}{n.note.Title, n.p.canonical(), isoDate(n.note.Date), n.p.l.tag(), n.note.Description, siteName})
	if err != nil {
		return nil, err
	}
	var b bytes.Buffer
	fmt.Fprintf(&b, "---\n%s---\n\n# %s\n\n", front, n.note.Title)
	b.Write(n.note.Source)
	return b.Bytes(), nil
}

func (n markdownNote) file() templ.Component {
	return file(func(context.Context) ([]byte, error) { return n.markdown() })
}

// llmsTxt is the index of the site for language models. See llmstxt.org.
func llmsTxt(notes []markdownNote) templ.Component {
	var b strings.Builder
	fmt.Fprintf(&b, "# %s\n\n", siteName)
	b.WriteString("> Personal website of Vinicius Hespanha, a software engineer in São Paulo, Brazil, " +
		"with notes on building small, understandable software. Written in English, " +
		"with some notes also in Brazilian Portuguese.\n\n")
	b.WriteString("Every note is also published as Markdown, at its URL with `.md` in place of the " +
		"trailing slash. [llms-full.txt](" + origin + "/llms-full.txt) holds every note in one file.\n")
	for _, l := range locales {
		var lines []string
		for _, n := range notes {
			if n.p.l == l {
				lines = append(lines, fmt.Sprintf("- [%s](%s): %s", n.note.Title, origin+markdownURL(n.p), n.note.Description))
			}
		}
		if len(lines) == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n## %s (%s)\n\n%s\n", l.notesTitle(), l.name(), strings.Join(lines, "\n"))
	}
	b.WriteString("\n## Optional\n\n")
	fmt.Fprintf(&b, "- [Home](%s/): Who I am and how to reach me.\n", origin)
	fmt.Fprintf(&b, "- [Privacy](%s/privacy/): How the site handles personal data.\n", origin)
	fmt.Fprintf(&b, "- [RSS feed](%s/feed.xml): New notes.\n", origin)
	return constant([]byte(b.String()))
}

// llmsFullTxt holds every note in full, in markdown.
func llmsFullTxt(notes []markdownNote) templ.Component {
	return file(func(context.Context) ([]byte, error) {
		var b bytes.Buffer
		fmt.Fprintf(&b, "# %s: all notes\n\n", siteName)
		for _, n := range notes {
			md, err := n.markdown()
			if err != nil {
				return nil, err
			}
			b.WriteString("\n")
			b.Write(md)
		}
		return b.Bytes(), nil
	})
}

// robotsTxt points crawlers at the sitemap. Cloudflare adds the rules for AI
// crawlers that are set in its dashboard above these lines.
func robotsTxt() templ.Component {
	return constant([]byte("Sitemap: " + origin + "/sitemap.xml\n"))
}

// securityTxt tells researchers how to report vulnerabilities (RFC 9116). It
// expires a year after the build, so it stays valid as long as the site is
// deployed at least once a year.
func securityTxt(now time.Time) templ.Component {
	expires := now.UTC().Truncate(24*time.Hour).AddDate(1, 0, 0)
	return constant(fmt.Appendf(nil, "Contact: mailto:mail@vhespanha.dev\n"+
		"Expires: %s\n"+
		"Preferred-Languages: en, pt\n"+
		"Canonical: %s/.well-known/security.txt\n", expires.Format(time.RFC3339), origin))
}

// apiCatalog lists the machine-readable resources of the site (RFC 9727).
func apiCatalog() templ.Component {
	type link struct {
		Href     string `json:"href"`
		Type     string `json:"type"`
		Hreflang string `json:"hreflang,omitempty"`
	}
	var feeds []link
	for _, l := range locales {
		feeds = append(feeds, link{origin + localeURL(l, "/feed.xml"), "application/rss+xml", l.tag()})
	}
	return jsonFile(map[string]any{
		"linkset": []any{map[string]any{
			"anchor":      origin + "/",
			"describedby": []link{{Href: origin + "/llms.txt", Type: "text/markdown"}},
			"sitemap":     []link{{Href: origin + "/sitemap.xml", Type: "application/xml"}},
			"alternate":   feeds,
		}},
	})
}

// webManifest describes the site in l to browsers that install it.
func webManifest(l locale) templ.Component {
	return file(func(ctx context.Context) ([]byte, error) {
		type icon struct {
			Src     string `json:"src"`
			Sizes   string `json:"sizes"`
			Type    string `json:"type"`
			Purpose string `json:"purpose"`
		}
		var icons []icon
		for _, i := range []icon{
			{"/assets/icon-192.png", "192x192", "image/png", "any"},
			{"/assets/icon-512.png", "512x512", "image/png", "any"},
			{"/assets/icon-maskable-512.png", "512x512", "image/png", "maskable"},
		} {
			src, err := asset(ctx, i.Src)
			if err != nil {
				return nil, err
			}
			i.Src = src
			icons = append(icons, i)
		}
		start := localeURL(l, "/")
		return json.MarshalIndent(map[string]any{
			"id":               start,
			"name":             siteName,
			"short_name":       "vhespanha",
			"description":      l.homeDescription(),
			"lang":             l.tag(),
			"dir":              "ltr",
			"start_url":        start,
			"scope":            "/",
			"display":          "minimal-ui",
			"background_color": "#f4f4f4",
			"theme_color":      "#f4f4f4",
			"icons":            icons,
		}, "", "  ")
	})
}

// noVarySearch lists the query parameters that never change a response, so
// that caches and prefetches can ignore them.
const noVarySearch = `params=("utm_source" "utm_medium" "utm_campaign" "utm_content" "utm_term" "gclid" "fbclid" "msclkid" "mc_cid" "mc_eid" "ref"), key-order`

// contentSecurityPolicy allows only the site's own files and its inline
// scripts, and turns on Trusted Types, which the scripts never need.
func contentSecurityPolicy() string {
	scripts := []string{"'self'"}
	for _, js := range inlineScripts {
		scripts = append(scripts, scriptHash(js))
	}
	return strings.Join([]string{
		"default-src 'none'",
		"script-src " + strings.Join(scripts, " "),
		"style-src 'self'",
		"img-src 'self'",
		"font-src 'self'",
		"connect-src 'self'",
		"manifest-src 'self'",
		"base-uri 'none'",
		"form-action 'none'",
		"frame-ancestors 'none'",
		"upgrade-insecure-requests",
		"require-trusted-types-for 'script'",
		"trusted-types 'none'",
	}, "; ")
}

// headers is Cloudflare's _headers file: response headers by URL pattern.
func headers() templ.Component {
	type rule struct {
		pattern string
		headers [][2]string
	}
	// Every response links to the site's machine-readable files. Pages under a
	// language prefix add the feed in their language.
	links := []string{
		`</llms.txt>; rel="describedby"; type="text/markdown"`,
		`</sitemap.xml>; rel="sitemap"; type="application/xml"`,
		`</.well-known/api-catalog>; rel="api-catalog"; type="application/linkset+json"`,
		feedLink(english{}),
	}
	rules := []rule{
		{"/*", [][2]string{
			{"Strict-Transport-Security", "max-age=63072000; includeSubDomains"},
			{"X-Content-Type-Options", "nosniff"},
			{"X-Frame-Options", "DENY"},
			{"Content-Security-Policy", contentSecurityPolicy()},
			{"Referrer-Policy", "strict-origin-when-cross-origin"},
			{"Permissions-Policy", "accelerometer=(), browsing-topics=(), camera=(), display-capture=(), geolocation=(), gyroscope=(), hid=(), magnetometer=(), microphone=(), midi=(), payment=(), publickey-credentials-get=(), screen-wake-lock=(), serial=(), usb=(), xr-spatial-tracking=()"},
			{"Cross-Origin-Opener-Policy", "same-origin"},
			{"Cross-Origin-Embedder-Policy", "require-corp"},
			{"Cross-Origin-Resource-Policy", "same-origin"},
			{"No-Vary-Search", noVarySearch},
			{"Link", strings.Join(links, ", ")},
		}},
		// Every file under /assets/ has a name that changes with its contents.
		{"/assets/*", [][2]string{{"Cache-Control", "public, max-age=31536000, immutable"}}},
		// Keep the workers.dev and preview URLs of the Worker out of search.
		{"https://:worker.:subdomain.workers.dev/*", [][2]string{{"X-Robots-Tag", "noindex"}}},
		{"/.well-known/api-catalog", [][2]string{{"Content-Type", "application/linkset+json"}, cors}},
		{"/llms.txt", [][2]string{cors}},
		{"/llms-full.txt", [][2]string{cors}},
		{"/sitemap.xml", [][2]string{cors}},
	}
	for _, l := range locales {
		feed := localeURL(l, "/feed.xml")
		if l.key() != "" {
			rules = append(rules, rule{localeURL(l, "/*"), [][2]string{{"Link", feedLink(l)}}})
		}
		rules = append(rules,
			rule{feed, [][2]string{{"Content-Type", "application/rss+xml; charset=utf-8"}, cors}},
			rule{localeURL(l, "/notes/:slug/"), [][2]string{
				{"Link", fmt.Sprintf(`<%s>; rel="alternate"; type="text/markdown"`, localeURL(l, "/notes/:slug.md"))},
			}},
			rule{localeURL(l, "/notes/:slug.md"), [][2]string{{"Content-Type", "text/markdown; charset=utf-8"}, cors}},
		)
	}
	var b strings.Builder
	for _, r := range rules {
		b.WriteString(r.pattern)
		b.WriteString("\n")
		for _, h := range r.headers {
			fmt.Fprintf(&b, "  %s: %s\n", h[0], h[1])
		}
		b.WriteString("\n")
	}
	return constant([]byte(b.String()))
}

var cors = [2]string{"Access-Control-Allow-Origin", "*"}

// feedLink is a Link header value pointing at the feed in l.
func feedLink(l locale) string {
	return fmt.Sprintf(`<%s>; rel="alternate"; type="application/rss+xml"; hreflang="%s"`, localeURL(l, "/feed.xml"), l.tag())
}

// redirects is Cloudflare's _redirects file. It sends each page's URL without
// its trailing slash to the page with a permanent redirect, where Cloudflare
// on its own would send a temporary one.
func redirects(listings []listing) templ.Component {
	var b strings.Builder
	paths := make([]string, 0, len(listings))
	for _, s := range listings {
		paths = append(paths, localeURL(s.p.l, s.p.path))
	}
	slices.Sort(paths)
	for _, p := range slices.Compact(paths) {
		if p == "/" {
			continue
		}
		fmt.Fprintf(&b, "%s %s 301\n", strings.TrimSuffix(p, "/"), p)
	}
	return constant([]byte(b.String()))
}
