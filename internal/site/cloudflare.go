package site

import (
	"slices"
	"strings"

	"github.com/a-h/templ"
)

// headers is Cloudflare's _headers file: response headers by URL pattern.
func headers() templ.Component {
	type loc struct{ Prefix, Tag string }
	data := struct {
		// ScriptHashes are the CSP sources of the inline scripts.
		ScriptHashes []string
		// Locales hold the URL prefix of each language, the first of which
		// is the default.
		Locales []loc
	}{}
	for _, js := range inlineScripts {
		data.ScriptHashes = append(data.ScriptHashes, scriptHash(js))
	}
	for _, l := range locales {
		data.Locales = append(data.Locales, loc{localeURL(l, ""), l.tag()})
	}
	return textFile("_headers.tmpl", data)
}

// redirects is Cloudflare's _redirects file, which adds the trailing slash to
// the URL of each page listed.
func redirects(listings []listing) templ.Component {
	type redirect struct{ From, To string }
	paths := make([]string, 0, len(listings))
	for _, s := range listings {
		paths = append(paths, localeURL(s.p.l, s.p.path))
	}
	slices.Sort(paths)
	var rs []redirect
	for _, p := range slices.Compact(paths) {
		if p != "/" {
			rs = append(rs, redirect{strings.TrimSuffix(p, "/"), p})
		}
	}
	return textFile("_redirects.tmpl", rs)
}
