// Order status page. The server owns the status; this only counts the
// reservation down and reloads the page once the polled status changes.
document.addEventListener("alpine:init", () => {
  window.Alpine.data("orderStatus", (options = {}) => ({
    status: options.status || "",
    expiresAt: options.expiresAt ? Date.parse(options.expiresAt) : null,
    remaining: 0,
    countdown: "",

    init() {
      this.tick();
      setInterval(() => this.tick(), 1000);
      setInterval(() => this.poll(), 10000);
    },

    tick() {
      if (!this.expiresAt) {
        this.remaining = 0;
        this.countdown = "";
        return;
      }
      const left = Math.max(0, Math.floor((this.expiresAt - Date.now()) / 1000));
      this.remaining = left;
      const minutes = String(Math.floor(left / 60)).padStart(2, "0");
      const seconds = String(left % 60).padStart(2, "0");
      this.countdown = `${minutes}:${seconds}`;
    },

    async poll() {
      if (!options.statusUrl) {
        return;
      }
      try {
        const response = await fetch(options.statusUrl, {
          headers: { "X-Requested-With": "fetch" },
        });
        if (!response.ok) {
          return;
        }
        const data = await response.json();
        if (data.status && data.status !== this.status) {
          window.location.reload();
        }
      } catch {
        // A failed poll is not worth reporting: the next one is ten seconds away.
      }
    },
  }));
});
