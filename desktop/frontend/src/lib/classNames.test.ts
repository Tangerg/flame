import { describe, expect, it } from "vitest";
import { cn } from "./classNames";

describe("cn", () => {
  it("keeps custom type size and text colour as independent properties", () => {
    expect(cn("text-cta-text", "text-ui-md")).toBe("text-cta-text text-ui-md");
    expect(cn("text-fg-soft", "text-display-lg")).toBe("text-fg-soft text-display-lg");
  });

  it("still resolves conflicts within each property", () => {
    expect(cn("text-ui-sm", "text-prose")).toBe("text-prose");
    expect(cn("text-fg-muted", "text-fg")).toBe("text-fg");
  });

  // Our steps declare a size and a tracking and no line height, so a following size must not
  // take a leading with it. It used to: `Button` lost the `leading-tight` in its cva base to
  // the `text-ui-*` in its size variant, which left every button on the body's PROSE rhythm
  // and its box shorter than its own line at the largest UI text; the transcript lost
  // `leading-prose` the same way. Order-dependent, and silent in both directions.
  it("never lets a type step swallow a leading", () => {
    expect(cn("leading-tight", "text-ui-sm")).toBe("leading-tight text-ui-sm");
    expect(cn("leading-prose", "text-prose")).toBe("leading-prose text-prose");
    expect(cn("leading-tight", "text-display-md")).toBe("leading-tight text-display-md");
  });

  it("still resolves one leading against another", () => {
    expect(cn("leading-body", "leading-prose")).toBe("leading-prose");
  });
});
