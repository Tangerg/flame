const IS_MAC = typeof navigator !== "undefined" && /Mac|iPhone|iPod|iPad/.test(navigator.platform);

// Maps, not objects: indexed by a segment of a plugin-written combo string, and an object
// answers `constructor` with a FUNCTION that would be returned as the glyph.
//
// This is the ONE place a modifier SPELLING is written down; the three tables below are
// keyed on the canonical form and carry four rows each. They used to carry every spelling
// themselves, and the copies had already drifted apart: `control` was in neither glyph
// table, so it printed as the word "Control" instead of ⌃, and `meta` labelled itself "Win"
// on Windows while the dispatcher fired it on Control.
const MODIFIER_ALIAS = new Map([
  ["cmd", "mod"],
  ["meta", "mod"],
  ["mod", "mod"],
  ["ctrl", "ctrl"],
  ["control", "ctrl"],
  ["shift", "shift"],
  ["alt", "alt"],
  ["option", "alt"],
]);

/** Cmd/meta fold to "mod" so a registration is cross-platform by default. An unmapped
 *  segment passes through unchanged, which keeps a literal "ctrl+k" distinct from "mod+k". */
function canonicalModifier(part: string): string {
  const lower = part.trim().toLowerCase();
  return MODIFIER_ALIAS.get(lower) ?? lower;
}

// Matches the common docs convention, e.g. "mod+shift+k".
const MODIFIER_ORDER = ["mod", "ctrl", "alt", "shift"] as const;
const CANONICAL_MODIFIERS = new Set<string>(MODIFIER_ORDER);

/**
 * "Cmd+K" / "cmd+K" / "Mod+k" -> "mod+k". Leftmost segments are modifiers; the last is the
 * key. Applied on both contribute and lookup so a registration and a keydown always agree.
 *
 * A segment this does not recognise SURVIVES, after the ones it does. Sorting by the known
 * order alone silently deleted it, and these combos are the dedup key of a single-keyed
 * extension point — so one typo ("shft+k") resolved to "k" and shadowed whatever legitimate
 * bare-key shortcut was already registered there.
 */
export function normalizeCombo(combo: string): string {
  const parts = combo.split("+").map((part) => part.trim().toLowerCase());
  const key = parts.pop() ?? "";
  const modifiers = [...new Set(parts.map(canonicalModifier))];
  return [
    ...MODIFIER_ORDER.filter((modifier) => modifiers.includes(modifier)),
    ...modifiers.filter((modifier) => !CANONICAL_MODIFIERS.has(modifier)),
    key,
  ].join("+");
}

const MAC_GLYPHS = new Map([
  ["mod", "⌘"],
  ["ctrl", "⌃"],
  ["shift", "⇧"],
  ["alt", "⌥"],
]);

const PC_LABELS = new Map([
  ["mod", "Ctrl"],
  ["ctrl", "Ctrl"],
  ["shift", "Shift"],
  ["alt", "Alt"],
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
  const modifier = (IS_MAC ? MAC_GLYPHS : PC_LABELS).get(canonicalModifier(lower));
  if (modifier) return modifier;
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
  ["ctrl", "Control"],
  ["alt", "Alt"],
  ["shift", "Shift"],
]);

function dispatchKey(key: string): string {
  if (/^[a-z]$/i.test(key)) return `Key${key.toUpperCase()}`;
  if (/^[0-9]$/.test(key)) return `Digit${key}`;
  return key;
}

/** Letters and digits become PHYSICAL key codes: `KeyboardEvent.key` carries whatever the
 *  active layout prints, so ⌘K under Cyrillic reports `"к"` and matches no registration.
 *  The key segment keeps its spelling — tinykeys names `Escape` and `Enter` in full. */
export function dispatchBinding(combo: string): string {
  return combo
    .trim()
    .split(/\s+/)
    .map((press) => {
      const parts = press.split("+").map((part) => part.trim());
      const key = parts.pop() ?? "";
      const modifiers = parts.map(
        (part) => DISPATCH_MODIFIERS.get(canonicalModifier(part)) ?? part,
      );
      return [...modifiers, dispatchKey(key)].join("+");
    })
    .join(" ");
}
