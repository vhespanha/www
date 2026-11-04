package site

import (
	"bytes"
	"context"
	"fmt"
	"strings"

	"github.com/a-h/templ"
	"go.vhespanha.dev/www/internal/ssg"
	"go.yaml.in/yaml/v3"
)

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

// llmsTxt is the index of the site for language models, with a section for
// the notes in each language.
func llmsTxt(notes []markdownNote) templ.Component {
	type entry struct{ Title, URL, Description string }
	type section struct {
		Title, Language string
		Notes           []entry
	}
	var sections []section
	for _, l := range locales {
		s := section{Title: l.notesTitle(), Language: l.name()}
		for _, n := range notes {
			if n.p.l == l {
				s.Notes = append(s.Notes, entry{n.note.Title, origin + markdownURL(n.p), n.note.Description})
			}
		}
		if len(s.Notes) > 0 {
			sections = append(sections, s)
		}
	}
	return textFile("llms.txt.tmpl", sections)
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
