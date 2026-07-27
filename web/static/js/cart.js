// Cart interactions. The server owns the cart; this only swaps the fragment it
// returns, so no cart state ever lives in the browser.
document.addEventListener("alpine:init", () => {
  window.Alpine.data("cart", () => ({
    async submit(event) {
      const form = event.target;
      const data = new FormData(form);
      const override = data.get("_method");
      data.delete("_method");

      const response = await fetch(form.action, {
        method: override || form.method || "POST",
        body: new URLSearchParams(data),
        headers: { "X-Requested-With": "fetch" },
      });

      const html = await response.text();
      const panel = document.getElementById("cart-panel");
      if (panel) {
        panel.innerHTML = html;
      }
    },
  }));
});
