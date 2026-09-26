import { describe, expect, it } from "vitest";
import { inputFromEditor, type EditorSnapshot } from "./editorContext";

describe("editor source context", () => {
  it("captures exact unsaved bytes and UTF-16 selection provenance before dispatch", () => {
    const snapshot: EditorSnapshot = {
      uri: "vscode-remote://ssh-remote+author/home/me/source.ts",
      documentVersion: 17,
      languageId: "typescript",
      dirty: true,
      selection: { start: { line: 1, character: 2 }, end: { line: 2, character: 5 } },
      text: "// unsaved\r\nconst title = '🔥';\r\n",
    };
    const input = inputFromEditor("Review this selection", snapshot);
    snapshot.documentVersion = 18;
    snapshot.text = "different unsaved edit";
    snapshot.selection.start.line = 5;
    expect(input[0]).toEqual({ type: "text", text: "Review this selection" });
    const block = input[1]!;
    if (block.type !== "text") throw new Error("expected source context");
    const captured = JSON.parse(block.text.slice(block.text.indexOf("\n") + 1));
    expect(captured.documentVersion).toBe(17);
    expect(captured.selection.start).toEqual({ line: 1, character: 2 });
    expect(captured.text).toBe("// unsaved\r\nconst title = '🔥';\r\n");
    expect(captured.uri).toBe("vscode-remote://ssh-remote+author/home/me/source.ts");
    expect(block.text).toContain("not the Runtime filesystem");
  });
});
