// Language preference. Loaded only on pages that exist in both languages, and
// everything it needs comes from the page: the alternate URL from <link>, the
// text from <template>. The site works in full without it.
//
// "lang" holds a language the reader explicitly chose. Dismissing the offer is
// kept apart from it, since declining a prompt does not ask to be redirected
// away from links in the other language later.

const CHOICE = "lang";
const DISMISSED = "lang-offer-dismissed";

function read(key) {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

function write(key, value) {
  try {
    localStorage.setItem(key, value);
  } catch {
    // Storage can be blocked. The choice then lasts only for this navigation.
  }
}

const primary = (tag) => tag.toLowerCase().split("-")[0];

// prefersOther reports whether the browser ranks the other language above this
// page's. A reader listing neither is left alone.
function prefersOther(other) {
  const current = primary(document.documentElement.lang);
  const target = primary(other.hreflang);
  const best = navigator.languages
    .map(primary)
    .find((lang) => lang === current || lang === target);
  return best === target;
}

document.addEventListener("click", (event) => {
  const choice = event.target.closest("a[data-lang-choice]");
  if (choice) write(CHOICE, choice.hreflang);
});

const other = document.querySelector("link[data-other]");
if (!read(CHOICE) && !read(DISMISSED) && prefersOther(other)) {
  const offer = document
    .getElementById("lang-offer")
    .content.firstElementChild.cloneNode(true);
  offer.querySelector("button").addEventListener("click", () => {
    write(DISMISSED, "1");
    offer.remove();
  });
  document.body.append(offer);
}
