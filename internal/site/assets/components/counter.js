// <demo-counter start="n"> enables its button, which counts clicks from n.
customElements.define(
  "demo-counter",
  class extends HTMLElement {
    connectedCallback() {
      const button = this.querySelector("button");
      let count = Number(this.getAttribute("start"));
      button.disabled = false;
      button.addEventListener("click", () => {
        button.textContent = `Count: ${++count}`;
      });
    }
  },
);
