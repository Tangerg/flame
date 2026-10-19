import type { Diff } from "@flame/runtime-contract/wire";

export const ACTIVE_DIFF_FILE =
  "desktop/frontend/src/plugins/builtin/shell/workbench/panel/DockResizer.tsx";

export const REVIEW_DIFF: Diff = {
  baseline: { type: "head", commit: "1234567890abcdef1234567890abcdef12345678" },
  files: [
    {
      path: ACTIVE_DIFF_FILE,
      status: "modified",
      added: 18,
      removed: 6,
      rows: [
        { type: "hunk", text: "@@ -104,8 +104,16 @@ export function DockResizer" },
        {
          type: "context",
          leftLine: 104,
          rightLine: 104,
          code: "const rail = railRef.current;",
        },
        {
          type: "deleted",
          leftLine: 105,
          code: "const width = clampDockWidth(persistedWidth, row.clientWidth);",
        },
        {
          type: "added",
          rightLine: 105,
          code: "const currentWidth = readDockWidth(row);",
        },
        {
          type: "added",
          rightLine: 106,
          code: "const nextWidth = clampDockWidth(currentWidth + delta, row.clientWidth);",
        },
        {
          type: "context",
          leftLine: 106,
          rightLine: 107,
          code: "row.style.setProperty(DOCK_WIDTH_PROPERTY, `${nextWidth}px`);",
        },
        {
          type: "added",
          rightLine: 108,
          code: 'rail.setAttribute("aria-valuenow", String(nextWidth));',
        },
      ],
    },
    {
      path: "runtime/protocol/session.go",
      status: "modified",
      added: 7,
      removed: 3,
      rows: [
        { type: "hunk", text: "@@ -48,5 +48,7 @@ func (session *Session) Commit" },
        {
          type: "context",
          leftLine: 48,
          rightLine: 48,
          code: "func (session *Session) Commit(ctx context.Context) error {",
        },
        {
          type: "deleted",
          leftLine: 49,
          code: "\treturn session.store.Save(ctx, session)",
        },
        {
          type: "added",
          rightLine: 49,
          code: "\tsnapshot := session.snapshot()",
        },
        {
          type: "added",
          rightLine: 50,
          code: "\treturn session.records.Commit(ctx, snapshot)",
        },
        { type: "context", leftLine: 50, rightLine: 51, code: "}" },
      ],
    },
  ],
};

export const BRANCH_REVIEW_DIFF: Diff = {
  baseline: { type: "mergeBase", commit: "abcdef1234567890abcdef1234567890abcdef12" },
  files: [
    REVIEW_DIFF.files![0]!,
    {
      path: "docs/layout/导航说明.md",
      previousPath: "docs/layout/旧导航.md",
      status: "renamed",
      added: 0,
      removed: 0,
      rows: [],
    },
    { path: "assets/flame-logo.png", status: "modified", binary: true, rows: [] },
  ],
};

export function scaledReview(fileCount: number): Diff {
  const files = Array.from({ length: fileCount }, (_, index) => {
    const long = index % 10 === 0;
    const rows: NonNullable<Diff["files"]>[number]["rows"] = [
      { type: "hunk", text: `@@ -1,24 +1,30 @@ export function module${index}` },
    ];
    for (let line = 1; line <= 24; line++) {
      const code = long
        ? `export const value${line} = ${JSON.stringify("x".repeat(320))};`
        : `export const value${line} = compute(${index}, ${line});`;
      rows.push({ type: "context", leftLine: line, rightLine: line, code });
      if (line % 4 === 0) {
        rows.push({ type: "deleted", leftLine: line, code: `${code} // before` });
        rows.push({ type: "added", rightLine: line, code: `${code} // after` });
      }
    }
    return {
      path: `src/generated/module-${String(index).padStart(3, "0")}.ts`,
      status: "modified" as const,
      added: 6,
      removed: 6,
      rows,
    };
  });
  return { baseline: REVIEW_DIFF.baseline, files };
}
