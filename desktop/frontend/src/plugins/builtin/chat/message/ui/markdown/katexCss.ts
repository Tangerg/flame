// ~30KB of @font-face and .katex* rules, so a dynamic import puts it in its own chunk the
// browser fetches only on the first math block. Only the STYLESHEET is lazy — rehype-katex
// itself is already bundled.

let loaded = false;

/** Idempotent and cheap to call repeatedly: a false positive like `$100` only triggers the
 *  load earlier, and the injection is one-shot. */
export function ensureKatexCss(): void {
  if (loaded) return;
  loaded = true;
  // Side-effect import — Vite emits this into its own asset chunk and
  // injects the resulting <link> when first evaluated.
  void import("./katexCssLoader");
}
