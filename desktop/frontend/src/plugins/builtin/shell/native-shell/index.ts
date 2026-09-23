import { definePlugin } from "@/plugins/sdk";

const EDITABLE = "input, textarea, [contenteditable='true']";

const WINDOW_INACTIVE_ATTR = "data-window-inactive";

export default definePlugin({
  name: "flame.builtin.native-shell",
  setup(ctx) {
    const onContextMenu = (e: MouseEvent) => {
      const target = e.target as HTMLElement | null;
      if (target?.closest(EDITABLE)) return;
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
