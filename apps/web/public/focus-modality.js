// Focus-ring modality — the pre-module half of the global focus rule in globals.css.
//
// The Wails WebView reports :focus-visible on a plain mouse click too, not just keyboard nav,
// so the accent ring flashed on every click. This tracks the last input device on
// `<html data-pointer>` (pointer by default) and flips to keyboard only on a nav key; the ring
// is gated on `html:not([data-pointer])` and its suppression on `html[data-pointer]`, so a ring
// shows only for genuine keyboard focus. Typing a character never resurrects it — nav keys only.
//
// A file rather than an inline script because the visual harness has its own entry, and it had
// no bootstrap at all: `data-pointer` was never set there, so `html:not([data-pointer])` matched
// forever and every fixture ran an app whose modality gate did not exist. What kept rings off
// the screenshots was a `outline: "none"` at each call site — which is also what kept them off
// the product, over the top of the rule here. Deleting those was what revealed this.
(function () {
  var root = document.documentElement;
  root.setAttribute("data-pointer", "");
  addEventListener(
    "pointerdown",
    function () {
      root.setAttribute("data-pointer", "");
    },
    true,
  );
  addEventListener(
    "keydown",
    function (e) {
      if (e.key === "Tab" || e.key.indexOf("Arrow") === 0) root.removeAttribute("data-pointer");
    },
    true,
  );
})();
