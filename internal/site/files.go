package site

import (
	"context"
	"embed"
	"encoding/json"
	"encoding/xml"
	"io"
	"text/template"
	"time"

	"github.com/a-h/templ"
)

// The files beside the pages are rendered in the files named for what they
// are for: feeds.go for crawlers and feed readers, agents.go for language
// models and cloudflare.go for the configuration Cloudflare reads from the
// output. This file holds what they share, and the files for browsers.
//
// Structured formats (XML, JSON, YAML) are rendered by their encoders, which
// escape what they write. Plain text formats are rendered from the templates
// in text/, which escape nothing, so their data must already be safe to write
// as is.

//go:embed static/favicon.ico
var faviconICO []byte

//go:embed text/*.tmpl
var textFS embed.FS

var textTemplates = template.Must(template.New("").
	Funcs(template.FuncMap{
		"origin":   func() string { return origin },
		"siteName": func() string { return siteName },
	}).
	ParseFS(textFS, "text/*.tmpl"))

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

// textFile returns a component that executes the template in text/ named name
// with data.
func textFile(name string, data any) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		return textTemplates.ExecuteTemplate(w, name, data)
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

// securityTxt tells researchers how to report vulnerabilities. It expires a
// year after the build, so it stays valid as long as the site is deployed at
// least once a year.
func securityTxt(now time.Time) templ.Component {
	expires := now.UTC().Truncate(24*time.Hour).AddDate(1, 0, 0)
	return textFile("security.txt.tmpl", struct{ Expires string }{expires.Format(time.RFC3339)})
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
