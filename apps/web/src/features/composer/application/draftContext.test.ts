import { expect, it } from "vitest";
import { formatFileReference } from "./draftContext";
it("quotes whitespace and escapes a quoted reference without interpreting ordinary draft text", () => {
  expect(formatFileReference("src/app.ts")).toBe("@src/app.ts");
  expect(formatFileReference("docs/my notes.md")).toBe('@"docs/my notes.md"');
});
