// The one `prefers-color-scheme` query in the app: the painter subscribes here, and
// `resolveThemeScheme` asks through the port this installs.

import { configureSystemAppearancePort } from "../application/ports/systemAppearance";

const media =
  typeof window !== "undefined" && typeof window.matchMedia === "function"
    ? window.matchMedia("(prefers-color-scheme: dark)")
    : null;

export function installSystemAppearance(): () => void {
  return configureSystemAppearancePort({
    // No media query (jsdom without matchMedia) reads as light — the same
    // answer a display with no dark mode gives.
    scheme: () => (media?.matches ? "dark" : "light"),
  });
}

/** Notify on OS appearance changes. No-op where the query is unavailable. */
export function subscribeSystemScheme(onChange: () => void): () => void {
  if (!media) return () => {};
  media.addEventListener("change", onChange);
  return () => media.removeEventListener("change", onChange);
}
