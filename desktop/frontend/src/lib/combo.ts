// Keeps the canonical combo for MATCHING and presents the keys the way the OS prints them.
// Platform detection is one-shot at module load — switching OS mid-session isn't a thing.

const IS_MAC = typeof navigator !== "undefined" && /Mac|iPhone|iPod|iPad/.test(navigator.platform);

// Maps, not objects: every table here is indexed by a segment of a combo string a
// plugin wrote, and an object literal answers `constructor` with a FUNCTION — which
// would then be returned as the glyph to render.
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

// Named keys whose display form doesn't depend on platform — arrows render as
// glyphs everywhere, "Escape" abbreviates to "Esc".
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
  // Capitalise multi-char keys (Enter, Tab, Space, …).
  return part.charAt(0).toUpperCase() + part.slice(1).toLowerCase();
}

/** Combo → display parts, e.g. "Mod+Shift+K" → ["⌘","⇧","K"]. */
export function splitCombo(combo: string): string[] {
  return combo.split("+").map(formatPart);
}

/** Combo → compact glyph string, e.g. "Mod+N" → "⌘N". */
export function comboGlyph(combo: string): string {
  return splitCombo(combo).join("");
}

// Canonical modifier → the name tinykeys matches. `$mod` resolves to Meta on
// Mac and Control elsewhere — narrower than "either one, on both platforms",
// which is what the hand-rolled dispatcher did and which made ⌃K on a Mac open
// the command palette. Cocoa has already spent that chord on kill-line.
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

/**
 * Letters and digits become PHYSICAL key codes, because a shortcut is a position on the
 * keyboard rather than a character: `KeyboardEvent.key` carries whatever the active layout
 * prints there, so ⌘K under Cyrillic reports `"к"` and matches no registration.
 *
 * Everything else passes through — tinykeys compares `KeyboardEvent.key` case-insensitively,
 * and a punctuation key has no code worth guessing at.
 */
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
