// Language preference. Loaded only on pages that exist in both languages, and
// everything it needs comes from the page: the alternate URL from <link>, the
// text from <template>. The site works in full without it.
//
// DONE is set once the reader has chosen a language or dismissed the offer,
// after which it is never shown again.

const DONE = "lang-offer-done";

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
  if (choice) write(DONE, "1");
});

const other = document.querySelector("link[data-other]");
if (!read(DONE) && prefersOther(other)) {
  offer();
}

// offer shows the prompt to read this page in the other language. It is a
// popover, whose button closes it by itself; browsers too old for popovers
// are not offered anything.
function offer() {
  const aside = document
    .getElementById("lang-offer-template")
    .content.firstElementChild.cloneNode(true);
  if (!aside.showPopover) return;
  const root = document.documentElement;
  aside.addEventListener("toggle", (event) => {
    if (event.newState !== "closed") return;
    write(DONE, "1");
    aside.remove();
    root.style.removeProperty("--offer-block-size");
  });
  const button = aside.querySelector("button");
  if (!("command" in button)) {
    // Invoker commands are newer than popovers.
    button.addEventListener("click", () => aside.hidePopover());
  }
  document.body.append(aside);
  aside.showPopover();
  // The page keeps room below its content for the offer, so that it never
  // hides a focused link. See lang.css.
  new ResizeObserver(([entry]) => {
    root.style.setProperty(
      "--offer-block-size",
      `${entry.borderBoxSize[0].blockSize}px`,
    );
  }).observe(aside);
}
