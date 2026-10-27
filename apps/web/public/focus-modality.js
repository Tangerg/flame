// WebKit can match :focus-visible after pointer input, so the shared ring also
// follows explicit input modality. Navigation and activation reveal it; ordinary
// typing and IME composition retain the current mode.
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
      if (e.isComposing || e.keyCode === 229) return;
      if (keyboardFocusKeys.includes(e.key)) root.removeAttribute("data-pointer");
    },
    true,
  );
})();
