import { describe, expect, it } from "vitest";
import { getCachedHighlight, setCachedHighlight } from "@/lib/highlight/shikiCache";
describe("highlight cache identity", () => {
  it("keeps distinct language, theme and source tuples apart even when their text overlaps", () => {
    setCachedHighlight("text", "github-dark", "some:github-dark:payload", "<first>");

    expect(getCachedHighlight("text:github-dark:some", "github-dark", "payload")).toBeUndefined();

    setCachedHighlight("text:github-dark:some", "github-dark", "payload", "<second>");
    expect(getCachedHighlight("text", "github-dark", "some:github-dark:payload")).toBe("<first>");
    expect(getCachedHighlight("text:github-dark:some", "github-dark", "payload")).toBe("<second>");
  });
});
