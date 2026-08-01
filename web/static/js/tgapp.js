// Telegram Mini App bootstrap.
//
// The Telegram client puts the launch payload in the URL fragment when it opens
// the web app, so nothing has to be loaded from telegram.org: the CSP of this
// project allows same-origin scripts only (tech.md §9.7). The fragment never
// reaches the server on its own, which is why this posts it to /tgapp/auth.
document.addEventListener("DOMContentLoaded", () => {
  const root = document.getElementById("tgapp-launch");
  if (!root) {
    return;
  }

  const status = root.querySelector("[data-launch-status]");
  const say = (message) => {
    if (status) {
      status.textContent = message;
    }
  };

  const launch = new URLSearchParams(window.location.hash.replace(/^#/, ""));
  const initData = launch.get("tgWebAppData");
  if (!initData) {
    say("Open this page from the Telegram bot.");
    return;
  }

  const body = new URLSearchParams();
  body.set("init_data", initData);
  body.set("csrf_token", root.dataset.csrfToken || "");
  const theme = launch.get("tgWebAppThemeParams");
  if (theme) {
    body.set("theme", theme);
  }

  fetch(root.dataset.authUrl, {
    method: "POST",
    headers: {
      "Content-Type": "application/x-www-form-urlencoded",
      "X-Requested-With": "fetch",
    },
    body: body.toString(),
    credentials: "same-origin",
  })
    .then((response) => response.json().then((payload) => ({ response, payload })))
    .then(({ response, payload }) => {
      if (!response.ok || !payload.redirect) {
        say(payload.error || "This Telegram account cannot open the admin panel.");
        return;
      }
      // replace, not assign: the launch payload is single use and must not stay
      // in the address bar or in a history entry the operator can go back to.
      window.location.replace(payload.redirect);
    })
    .catch(() => {
      say("Could not reach the panel. Try opening it again.");
    });
});
