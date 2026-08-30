import type { Plugin } from "vite";

// Every reicon icon module carries BOTH weights as template literals handed to
// `createIcon`, so a bundler cannot see that one is dead — the object is a value, not a
// namespace. `Icon` never passes `weight`, so `Filled` is ~93 KB of raw payload that is
// parsed before first paint and never drawn.
//
// The regex is anchored on the exact literal reicon generates. A library upgrade that
// changes that shape leaves the module untouched rather than corrupting it: the icon
// still renders, and `check:bundle` is what reports the lost saving.

const ICON_MODULE = /node_modules[/\\]reicon-react[/\\]icons[/\\][^/\\]+\.js$/;
const UNUSED_WEIGHT = /,\s*\n\s*F: `(?:[^`\\]|\\.)*`(?=\s*\n\}\);)/;

export function reiconOutlineOnly(): Plugin {
  return {
    name: "flame:reicon-outline-only",
    enforce: "pre",
    transform(code, id) {
      if (!ICON_MODULE.test(id) || !UNUSED_WEIGHT.test(code)) return null;
      return { code: code.replace(UNUSED_WEIGHT, ""), map: null };
    },
  };
}
