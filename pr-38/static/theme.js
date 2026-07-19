// SPDX-License-Identifier: GPL-2.0-or-later
// Client-only light/dark toggle. The server renders identically in both themes;
// this just remembers the viewer's choice. Progressive enhancement — the page
// is fully readable with JS disabled.
(function () {
  "use strict";
  var root = document.documentElement;
  var toggle = document.getElementById("themeToggle");
  var icon = document.getElementById("themeIcon");
  var KEY = "tally-theme";

  function prefersDark() {
    return window.matchMedia && window.matchMedia("(prefers-color-scheme: dark)").matches;
  }
  function apply(mode) {
    root.setAttribute("data-theme", mode);
    if (icon) icon.textContent = mode === "dark" ? "🌙" : "☀️";
  }

  var saved = null;
  try { saved = window.localStorage.getItem(KEY); } catch (e) {}
  apply(saved || (prefersDark() ? "dark" : "light"));

  if (toggle) {
    toggle.addEventListener("click", function () {
      var next = root.getAttribute("data-theme") === "dark" ? "light" : "dark";
      apply(next);
      try { window.localStorage.setItem(KEY, next); } catch (e) {}
    });
  }
})();
