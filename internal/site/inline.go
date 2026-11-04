package site

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"io"

	"github.com/a-h/templ"
)

// inlineScripts are the scripts written into pages rather than loaded from a
// file. The Content-Security-Policy allows exactly these, by hash, so each must
// be rendered byte for byte as it is here.
var inlineScripts = []string{languageRedirect}

// languageRedirect sends a reader who explicitly chose the other language to
// this page in that language. It only acts when arriving from elsewhere, so it
// never overrides a link followed within the site or traps the back button. It
// is inline so that it runs before the first paint. The rest of the behaviour
// lives in lang.js.
const languageRedirect = `(function () {
  try {
    var other = document.querySelector("link[data-other]");
    if (localStorage.getItem("lang") !== other.hreflang) return;
    var nav = performance.getEntriesByType("navigation")[0];
    if (nav && nav.type !== "navigate") return;
    if (document.referrer && new URL(document.referrer).origin === location.origin) return;
    location.replace(new URL(other.href).pathname + location.search + location.hash);
  } catch (e) {}
})();`

// inlineScript writes js in a <script> element.
func inlineScript(js string) templ.Component {
	return templ.ComponentFunc(func(_ context.Context, w io.Writer) error {
		_, err := io.WriteString(w, "<script>"+js+"</script>")
		return err
	})
}

// scriptHash returns the CSP source expression allowing the inline script js.
func scriptHash(js string) string {
	sum := sha256.Sum256([]byte(js))
	return "'sha256-" + base64.StdEncoding.EncodeToString(sum[:]) + "'"
}
