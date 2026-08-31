const IS_MAC = typeof navigator !== "undefined" && /Mac|iPhone|iPod|iPad/.test(navigator.platform);

// Maps, not objects: indexed by a segment of a plugin-written combo string, and an object
// answers `constructor` with a FUNCTION that would be returned as the glyph.
const MAC_GLYPHS = new Map([
  ["mod", "⌘"],
  ["cmd", "⌘"],
  ["ctrl", "⌃"],
  ["shift", "⇧"],
  ["alt", "⌥"],
  ["option", "⌥"],
  ["meta", "⌘"],
]);

const PC_LABELS = new Map([
  ["mod", "Ctrl"],
  ["cmd", "Ctrl"],
  ["ctrl", "Ctrl"],
  ["shift", "Shift"],
  ["alt", "Alt"],
  ["option", "Alt"],
  ["meta", "Win"],
]);

const NAMED_KEYS = new Map([
  ["escape", "Esc"],
  ["arrowup", "↑"],
  ["arrowdown", "↓"],
  ["arrowleft", "←"],
  ["arrowright", "→"],
]);

function formatPart(part: string): string {
  const lower = part.toLowerCase();
  const mod = (IS_MAC ? MAC_GLYPHS : PC_LABELS).get(lower);
  if (mod) return mod;
  const named = NAMED_KEYS.get(lower);
  if (named) return named;
  if (lower.length === 1) return lower.toUpperCase();
  return part.charAt(0).toUpperCase() + part.slice(1).toLowerCase();
}

export function splitCombo(combo: string): string[] {
  return combo.split("+").map(formatPart);
}

export function comboGlyph(combo: string): string {
  return splitCombo(combo).join("");
}

// `$mod` resolves to Meta on Mac and Control elsewhere — narrower than "either, on both",
// which made ⌃K open the command palette on a Mac where Cocoa owns that chord.
const DISPATCH_MODIFIERS = new Map([
  ["mod", "$mod"],
  ["cmd", "$mod"],
  ["meta", "$mod"],
  ["ctrl", "Control"],
  ["control", "Control"],
  ["alt", "Alt"],
  ["option", "Alt"],
  ["shift", "Shift"],
]);

function dispatchKey(key: string): string {
  if (/^[a-z]$/i.test(key)) return `Key${key.toUpperCase()}`;
  if (/^[0-9]$/.test(key)) return `Digit${key}`;
  return key;
}

/** Letters and digits become PHYSICAL key codes: `KeyboardEvent.key` carries whatever the
 *  active layout prints, so ⌘K under Cyrillic reports `"к"` and matches no registration. */
export function dispatchBinding(combo: string): string {
  return combo
    .trim()
    .split(/\s+/)
    .map((press) => {
      const parts = press.split("+").map((part) => part.trim());
      const key = parts.pop() ?? "";
      const modifiers = parts.map((part) => DISPATCH_MODIFIERS.get(part.toLowerCase()) ?? part);
      return [...modifiers, dispatchKey(key)].join("+");
    })
    .join(" ");
}
