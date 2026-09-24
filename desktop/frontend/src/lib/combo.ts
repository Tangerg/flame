const IS_MAC = typeof navigator !== "undefined" && /Mac|iPhone|iPod|iPad/.test(navigator.platform);

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

function canonicalModifier(part: string): string {
  const lower = part.trim().toLowerCase();
  return MODIFIER_ALIAS.get(lower) ?? lower;
}

const MODIFIER_ORDER = ["mod", "ctrl", "alt", "shift"] as const;
const CANONICAL_MODIFIERS = new Set<string>(MODIFIER_ORDER);

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

const ARIA_MODIFIERS = new Map([
  ["ctrl", "Control"],
  ["alt", "Alt"],
  ["shift", "Shift"],
]);

export function ariaKeyShortcuts(combo: string): string {
  const parts = normalizeCombo(combo).split("+");
  const key = parts.pop() ?? "";
  const named = key.length === 1 ? key.toUpperCase() : key.charAt(0).toUpperCase() + key.slice(1);
  const spell = (mod: string) =>
    [
      ...parts.map((part) => (part === "mod" ? mod : (ARIA_MODIFIERS.get(part) ?? part))),
      named,
    ].join("+");
  return parts.includes("mod") ? `${spell("Meta")} ${spell("Control")}` : spell("Meta");
}

const DISPATCH_MODIFIERS = new Map([
  ["mod", "$mod"],
  ["ctrl", "Control"],
  ["alt", "Alt"],
  ["shift", "Shift"],
]);

const DISPATCH_CODES = new Map([
  ["[", "BracketLeft"],
  ["]", "BracketRight"],
  ["\\", "Backslash"],
  [";", "Semicolon"],
  ["'", "Quote"],
  [",", "Comma"],
  [".", "Period"],
  ["/", "Slash"],
  ["-", "Minus"],
  ["=", "Equal"],
  ["`", "Backquote"],
]);

function dispatchKey(key: string): string {
  if (/^[a-z]$/i.test(key)) return `Key${key.toUpperCase()}`;
  if (/^[0-9]$/.test(key)) return `Digit${key}`;
  return DISPATCH_CODES.get(key) ?? key;
}

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

const KEY_FROM_CODE = new Map([...DISPATCH_CODES].map(([key, code]) => [code, key]));
const MODIFIER_KEYS = new Set(["Meta", "Control", "Alt", "Shift", "CapsLock", "Fn"]);

export function comboFromEvent(event: {
  code: string;
  key: string;
  metaKey: boolean;
  ctrlKey: boolean;
  altKey: boolean;
  shiftKey: boolean;
}): string | null {
  if (MODIFIER_KEYS.has(event.key)) return null;
  const key = /^Key[A-Z]$/.test(event.code)
    ? event.code.slice(3).toLowerCase()
    : /^Digit\d$/.test(event.code)
      ? event.code.slice(5)
      : (KEY_FROM_CODE.get(event.code) ?? event.key.toLowerCase());
  const modifiers = [
    (IS_MAC ? event.metaKey : event.ctrlKey) && "mod",
    IS_MAC && event.ctrlKey && "ctrl",
    event.altKey && "alt",
    event.shiftKey && "shift",
  ].filter((part): part is string => Boolean(part));
  return normalizeCombo([...modifiers, key].join("+"));
}
