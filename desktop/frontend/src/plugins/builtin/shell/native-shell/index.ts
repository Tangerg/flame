// Makes the webview behave like a window rather than a page.
//
// The default right-click menu is suppressed everywhere EXCEPT real text fields, where on
// WKWebView it is the native macOS edit menu. Base UI menus are unaffected: their React
// trigger runs before this document-level bubble listener, so `preventDefault` here only
// kills the browser menu where nothing is wired.

import { definePlugin } from "@/plugins/sdk";

const EDITABLE = "input, textarea, [contenteditable='true']";

/** Read from `document.hasFocus()` rather than tracked from events alone, so a window that
 *  opens behind another app is not painted as focused. The attribute marks the INACTIVE
 *  state: absence has to mean "coloured", or a platform that never reports focus — or this
 *  plugin failing to load — ships a window whose controls are permanently grey. */
const WINDOW_INACTIVE_ATTR = "data-window-inactive";

export default definePlugin({
  name: "flame.builtin.native-shell",
  setup(ctx) {
    const onContextMenu = (e: MouseEvent) => {
      const target = e.target as HTMLElement | null;
      if (target?.closest(EDITABLE)) return; // keep the system edit menu on inputs
      e.preventDefault();
    };
    document.addEventListener("contextmenu", onContextMenu);

    const syncFocus = () => {
      document.documentElement.toggleAttribute(WINDOW_INACTIVE_ATTR, !document.hasFocus());
    };
    syncFocus();
    addEventListener("focus", syncFocus);
    addEventListener("blur", syncFocus);

    ctx.cleanup(() => {
      document.removeEventListener("contextmenu", onContextMenu);
      removeEventListener("focus", syncFocus);
      removeEventListener("blur", syncFocus);
      document.documentElement.removeAttribute(WINDOW_INACTIVE_ATTR);
    });
  },
});
