let loaded = false;

export function ensureKatexCss(): void {
  if (loaded) return;
  loaded = true;
  void import("@/features/chat/message/ui/markdown/katexCssLoader");
}
