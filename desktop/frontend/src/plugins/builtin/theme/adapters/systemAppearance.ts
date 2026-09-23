import { configureSystemAppearancePort } from "../application/ports/systemAppearance";

const media =
  typeof window !== "undefined" && typeof window.matchMedia === "function"
    ? window.matchMedia("(prefers-color-scheme: dark)")
    : null;

export function installSystemAppearance(): () => void {
  return configureSystemAppearancePort({
    scheme: () => (media?.matches ? "dark" : "light"),
  });
}

export function subscribeSystemScheme(onChange: () => void): () => void {
  if (!media) return () => {};
  media.addEventListener("change", onChange);
  return () => media.removeEventListener("change", onChange);
}
