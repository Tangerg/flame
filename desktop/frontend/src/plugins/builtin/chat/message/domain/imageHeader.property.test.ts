import { describe, expect, it } from "vitest";
import { forEachSeed } from "@/test/arbitrary";
import { imageSizeFromBase64 } from "./imageHeader";

// A base64 payload from a model, read for its header alone. A reader that throws here takes
// the message card with it.

describe("the image header reader, over arbitrary payloads", () => {
  it("answers a size or null, never a partial one", () => {
    forEachSeed(600, (a) => {
      const payload = btoa(a.bytes(300));
      const size = imageSizeFromBase64(payload);
      if (size === null) return;
      expect(Number.isInteger(size.width)).toBe(true);
      expect(Number.isInteger(size.height)).toBe(true);
      expect(size.width).toBeGreaterThan(0);
      expect(size.height).toBeGreaterThan(0);
    });
  });

  it("does not mind a payload that is not base64 at all", () => {
    forEachSeed(400, (a) => {
      expect(() => imageSizeFromBase64(a.text())).not.toThrow();
    });
  });
});
