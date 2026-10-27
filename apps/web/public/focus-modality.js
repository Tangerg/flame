// WebKit can match :focus-visible after pointer input and omit it after shortcuts.
// The shared ring follows explicit input modality; ordinary typing and IME
// composition retain the current mode.
(function () {
  var root = document.documentElement;
  var keyboardFocusKeys = [
    "Tab",
    "ArrowUp",
    "ArrowDown",
    "ArrowLeft",
    "ArrowRight",
    "Home",
    "End",
    "PageUp",
    "PageDown",
    "Enter",
    " ",
    "Escape",
  ];
  var nonCommandKeys = [
    "Meta",
    "Control",
    "Alt",
    "Shift",
    "AltGraph",
    "Dead",
    "Process",
    "Unidentified",
  ];
  root.setAttribute("data-input-modality", "pointer");
  addEventListener(
    "pointerdown",
    function () {
      root.setAttribute("data-input-modality", "pointer");
    },
    true,
  );
  addEventListener(
    "keydown",
    function (e) {
      if (e.isComposing || e.keyCode === 229) return;
      var command =
        (e.metaKey || e.ctrlKey) &&
        !nonCommandKeys.includes(e.key) &&
        !e.getModifierState("AltGraph");
      if (keyboardFocusKeys.includes(e.key) || command) {
        root.setAttribute("data-input-modality", "keyboard");
      }
    },
    true,
  );
})();
