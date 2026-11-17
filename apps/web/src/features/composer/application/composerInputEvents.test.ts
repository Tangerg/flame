import { describe, expect, it } from "vitest";
import { LARGE_PASTE_CHARS } from "@/features/composer/domain/largePaste";
import {
  composerKeyBindingKey,
  composerPasteIntent,
} from "@/features/composer/application/composerInputEvents";
const image = new File(["image"], "shot.png", { type: "image/png" });

describe("composerPasteIntent", () => {
  it("prefers pasted images over clipboard text", () => {
    expect(composerPasteIntent([image], "x".repeat(LARGE_PASTE_CHARS), true)).toEqual({
      kind: "images",
      files: [image],
    });
  });

  it("rejects unsupported images without discarding usable clipboard text", () => {
    expect(composerPasteIntent([image], "", false)).toEqual({ kind: "unsupported-images" });
    expect(composerPasteIntent([image], "caption", false)).toEqual({ kind: "native" });
    const text = "x".repeat(LARGE_PASTE_CHARS);
    expect(composerPasteIntent([image], text, false)).toEqual({ kind: "large-text", text });
  });

  it("collapses large pasted text into a paste attachment", () => {
    const text = "x".repeat(LARGE_PASTE_CHARS);

    expect(composerPasteIntent([], text, false)).toEqual({ kind: "large-text", text });
  });

  it("leaves small text to the browser paste path", () => {
    expect(composerPasteIntent([], "small snippet", false)).toEqual({ kind: "native" });
  });
});

describe("composerKeyBindingKey", () => {
  it("uses the library grammar for physical keyboard events", () => {
    expect(composerKeyBindingKey(new KeyboardEvent("keydown", { key: "Enter" }))).toBe("Enter");
    expect(composerKeyBindingKey(new KeyboardEvent("keydown", { key: "ArrowUp" }))).toBe("ArrowUp");
  });
});
