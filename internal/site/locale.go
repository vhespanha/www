package site

import (
	"time"

	"github.com/a-h/templ"
)

// origin is the website's address, for the absolute URLs search engines require.
const origin = "https://vhespanha.dev"

// locales lists every language of the website.
var locales = []locale{english{}, portuguese{}}

// A locale is one language of the website and all of its base content.
//
// Text is reached through methods rather than string keys, so a missing
// translation is a compile error rather than a blank or an English fallback in
// production. Adding text means adding a method here, which forces every locale
// to provide it.
//
// Each method returns a whole sentence or a component, never a fragment to be
// joined with others, because word order differs between languages. Anything
// that varies with a number or a date takes it as an argument.
type locale interface {
	// key names the locale in URL prefixes and note file names. It is empty for
	// the original language, which has neither.
	key() string
	// tag is the locale's BCP 47 language tag.
	tag() string
	// name is the language's name in that language.
	name() string
	// other is the locale to offer as an alternative.
	other() locale

	homeTitle() string
	role() string
	openToRoles() string
	email() string
	intro() templ.Component

	notesTitle() string
	notesKicker() string
	allNotes() string
	readingTime(minutes int) string
	longDate(t time.Time) string
	// inEnglish marks a listed note as having no translation.
	inEnglish() string

	notFoundTitle() string
	notFoundBody() templ.Component

	languageLabel() string
	// offerText, offerAccept and offerDismiss make up the prompt offered to
	// readers who prefer this locale while on a page in another.
	offerText() string
	offerAccept() string
	offerDismiss() string
}

// localeURL returns the URL path of a page in the locale l, given its path
// without a language prefix.
func localeURL(l locale, path string) string {
	if l.key() == "" {
		return path
	}
	return "/" + l.key() + path
}
