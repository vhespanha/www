package site

import (
	"context"
	"embed"
	"encoding/xml"
	"io"
	"text/template"

	"github.com/a-h/templ"
)

// The files beside the pages are rendered in the files named for what they
// are for: feeds.go for crawlers and feed readers, agents.go for language
// models and cloudflare.go for the configuration Cloudflare reads from the
// output. This file holds what they share, and the files for browsers.
//
// Structured formats (XML, YAML) are rendered by their encoders, which
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
