import { definePlugin } from "@/extensions/definePlugin";
const EDITABLE = "input, textarea, [contenteditable='true']";

const WINDOW_INACTIVE_ATTR = "data-window-inactive";

export default definePlugin({
  name: "flame.builtin.native-shell",
  setup(ctx) {
    const onContextMenu = (event: MouseEvent) => {
      if (event.defaultPrevented) return;
      const target = event.target;
      if (target instanceof Element && target.closest(EDITABLE)) return;
      event.preventDefault();
    };
    // Element and document menus can claim this event before the shell fallback.
    window.addEventListener("contextmenu", onContextMenu);
    ctx.lifetime.defer(() => window.removeEventListener("contextmenu", onContextMenu));

    const syncFocus = () => {
      document.documentElement.toggleAttribute(WINDOW_INACTIVE_ATTR, !document.hasFocus());
    };
    syncFocus();
    ctx.lifetime.defer(() => document.documentElement.removeAttribute(WINDOW_INACTIVE_ATTR));
    addEventListener("focus", syncFocus);
    ctx.lifetime.defer(() => removeEventListener("focus", syncFocus));
    addEventListener("blur", syncFocus);
    ctx.lifetime.defer(() => removeEventListener("blur", syncFocus));
  },
});
