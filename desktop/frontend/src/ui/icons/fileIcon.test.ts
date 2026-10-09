import { describe, expect, it } from "vitest";
import { fileIconName } from "./fileIcon";

describe("file icon semantics", () => {
  it.each([
    ["src/Button.tsx", "file-code"],
    ["src/main.go", "file-code"],
    ["Dockerfile", "file-code"],
    ["README.md", "file-text"],
    ["notes.TXT", "file-text"],
    ["assets/logo.PNG", "file-image"],
    ["assets/diagram.svg", "file-image"],
    ["reports/plan.PDF", "file-pdf"],
    ["build/artifacts.tar.gz", "file-archive"],
    ["backup.7z", "file-archive"],
    ["audio/voice.opus", "file-music"],
    ["video/demo.webm", "file-video"],
    ["data/results.csv", "file-spreadsheet"],
    ["data/budget.xlsx", "file-spreadsheet"],
    ["package.json", "file-braces"],
    ["settings.jsonc", "file-braces"],
    ["records.ndjson", "file-braces"],
    ["data/unknown.blob", "file"],
    ["LICENSE", "file"],
    [".gitignore", "file"],
    ["folder.tsx/unknown", "file"],
    ["payload.constructor", "file"],
  ])("represents %s with %s", (path, icon) => {
    expect(fileIconName(path)).toBe(icon);
  });
});
