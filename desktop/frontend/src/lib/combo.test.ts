import { describe, expect, it } from "vitest";
import { matchKeybindingPress, parseKeybinding } from "tinykeys";

import { comboGlyph, dispatchBinding, normalizeCombo } from "./combo";

// A keydown as the browser reports it. `key` is what the active layout prints
// at that position; `code` is the position itself.
function keydown(key: string, code: string, modifiers: string[] = []): KeyboardEvent {
  return {
    key,
    code,
    getModifierState: (modifier: string) => modifiers.includes(modifier),
  } as unknown as KeyboardEvent;
}

function matches(combo: string, event: KeyboardEvent): boolean {
  const [press] = parseKeybinding(dispatchBinding(combo));
  return matchKeybindingPress(event, press!);
}

describe("normalizeCombo", () => {
  it("folds every accepted spelling to one canonical form", () => {
    for (const combo of ["Cmd+K", "cmd+k", "Meta+K", "Mod+k"]) {
      expect(normalizeCombo(combo)).toBe("mod+k");
    }
    expect(normalizeCombo("Control+K")).toBe("ctrl+k");
    expect(normalizeCombo("Option+K")).toBe("alt+k");
  });

  it("orders modifiers so a registration and a keydown spell the same string", () => {
    expect(normalizeCombo("Shift+Alt+Ctrl+Mod+K")).toBe("mod+ctrl+alt+shift+k");
    expect(normalizeCombo("Mod+Ctrl+Alt+Shift+K")).toBe("mod+ctrl+alt+shift+k");
  });

  it("keeps ctrl+k distinct from mod+k", () => {
    expect(normalizeCombo("ctrl+k")).not.toBe(normalizeCombo("mod+k"));
  });

  // These strings are the dedup key of a single-keyed extension point, so a dropped
  // modifier is not a cosmetic loss: "shft+k" became "k" and shadowed the bare-key
  // shortcut already registered there.
  it("keeps a segment it does not recognise rather than resolving to the bare key", () => {
    expect(normalizeCombo("hyper+k")).toBe("hyper+k");
    expect(normalizeCombo("shft+k")).not.toBe(normalizeCombo("k"));
    expect(normalizeCombo("Mod+hyper+K")).toBe("mod+hyper+k");
  });
});

describe("the modifier vocabulary", () => {
  // One table, so the label and the handler cannot disagree about what a spelling means.
  // Both of these used to: `control` printed as the word, and `meta` said "Win" on a
  // platform where the dispatcher fired it on Control.
  it("gives every spelling of one modifier the same dispatch and the same glyph", () => {
    for (const [a, b] of [
      ["cmd+k", "mod+k"],
      ["meta+k", "mod+k"],
      ["control+k", "ctrl+k"],
      ["option+k", "alt+k"],
    ]) {
      expect(dispatchBinding(a!)).toBe(dispatchBinding(b!));
      expect(comboGlyph(a!)).toBe(comboGlyph(b!));
    }
  });

  // Without their own table these read as title case — "Escape", "Arrowup" — which is what a
  // menu hint would print beside the command.
  it("prints a named key the way a keyboard does", () => {
    expect(comboGlyph("Escape")).toBe("Esc");
    expect(comboGlyph("ArrowUp")).toBe("↑");
    expect(comboGlyph("Mod+ArrowLeft")).toContain("←");
  });
});

describe("dispatchBinding", () => {
  it("names the physical key for letters and digits", () => {
    expect(dispatchBinding("Mod+K")).toBe("$mod+KeyK");
    expect(dispatchBinding("Mod+Shift+L")).toBe("$mod+Shift+KeyL");
    expect(dispatchBinding("Alt+3")).toBe("Alt+Digit3");
  });

  it("passes named keys through for tinykeys to match case-insensitively", () => {
    expect(dispatchBinding("Escape")).toBe("Escape");
    expect(dispatchBinding("Mod+Enter")).toBe("$mod+Enter");
    expect(dispatchBinding("Mod+Shift+Backspace")).toBe("$mod+Shift+Backspace");
  });

  it("maps every modifier alias the registry accepts", () => {
    expect(dispatchBinding("cmd+k")).toBe("$mod+KeyK");
    expect(dispatchBinding("meta+k")).toBe("$mod+KeyK");
    expect(dispatchBinding("control+k")).toBe("Control+KeyK");
    expect(dispatchBinding("option+k")).toBe("Alt+KeyK");
  });

  it("keeps a space-separated sequence a sequence", () => {
    expect(dispatchBinding("Mod+K Mod+S")).toBe("$mod+KeyK $mod+KeyS");
    expect(parseKeybinding(dispatchBinding("Mod+K Mod+S"))).toHaveLength(2);
  });
});

describe("a letter shortcut under a non-US keyboard layout", () => {
  // The bug this replaced: ⌃K on a Cyrillic layout reports key "к", so a
  // dispatcher comparing against `KeyboardEvent.key` looked up "ctrl+к" and
  // found nothing. Every letter shortcut in the app was dead on that layout.
  const cyrillicK = keydown("к", "KeyK", ["Control"]);

  it("matches the binding we now emit", () => {
    expect(matches("Ctrl+K", cyrillicK)).toBe(true);
  });

  it("would not have matched one written against the printed character", () => {
    const [press] = parseKeybinding("Control+k");
    expect(matchKeybindingPress(cyrillicK, press!)).toBe(false);
  });

  it("still matches a US layout, where key and code agree", () => {
    expect(matches("Ctrl+K", keydown("k", "KeyK", ["Control"]))).toBe(true);
  });

  // `KeyboardEvent.key` for ⌘⇧] is `}`, on every layout there is, so a binding spelled with
  // the character it prints matches nothing. Letters do not need this — Shift+b reports `B`.
  it("dispatches punctuation by its physical code, not the glyph a modifier rewrites", () => {
    expect(dispatchBinding("Mod+Shift+]")).toBe("$mod+Shift+BracketRight");
    expect(dispatchBinding("Mod+Shift+[")).toBe("$mod+Shift+BracketLeft");
    expect(dispatchBinding("Mod+[")).toBe("$mod+BracketLeft");
    expect(dispatchBinding("Mod+k")).toBe("$mod+KeyK");
    expect(dispatchBinding("Escape")).toBe("Escape");
  });
});
