package site

import (
	"slices"
	"strings"

	"github.com/a-h/templ"
)

// headers is Cloudflare's _headers file: response headers by URL pattern.
func headers() templ.Component {
	// The URL prefix of each language.
	var prefixes []string
	for _, l := range locales {
		prefixes = append(prefixes, localeURL(l, ""))
	}
	return textFile("_headers.tmpl", prefixes)
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
