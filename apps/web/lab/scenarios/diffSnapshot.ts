import type { Diff, DiffMode, WorkspaceFileChange } from "@flame/runtime-client/wire";

export const ACTIVE_DIFF_FILE = "apps/web/src/features/shell/workbench/panel/DockResizer.tsx";
type VisualRead = Extract<Diff, { type: "visual" }>;
const BUDGET = { files: 5_000, rows: 5_000, bytes: 64 << 20 };
const SDK_FILE = "packages/go-sdk/protocol/session.go";
const DOCK_LINES = [
  "@@ -104,8 +104,20 @@ export function DockResizer",
  " const rail = railRef.current;",
  "-const width = clampDockWidth(persistedWidth, row.clientWidth);",
  "-const origin = event.clientX;",
  "-let dragging = true;",
  '-window.addEventListener("pointermove", move);',
  '-window.addEventListener("pointerup", finish);',
  "-row.style.setProperty(DOCK_WIDTH_PROPERTY, `${width}px`);",
  "+const currentWidth = readDockWidth(row);",
  "+const nextWidth = clampDockWidth(currentWidth + delta, row.clientWidth);",
  "+row.style.setProperty(DOCK_WIDTH_PROPERTY, `${nextWidth}px`);",
  '+rail.setAttribute("aria-valuenow", String(nextWidth));',
  "+const origin = captureCurrentScope();",
  "+const session = new PointerSession(event, rail, {",
  "+  move(delta) {",
  "+    if (!origin.current()) return;",
  "+    preview(currentWidth + delta.x);",
  "+  },",
  "+  complete() {",
  "+    if (origin.current()) commit(nextWidth);",
  "+  },",
  "+  cancel() {",
  "+    if (origin.current()) preview(currentWidth);",
  "+  },",
  "+  retire() {},",
  "+});",
  " return session;",
];
const SDK_LINES = [
  "@@ -48,5 +48,9 @@ func (session *Session) Commit",
  " func (session *Session) Commit(ctx context.Context) error {",
  "-\tvalues := session.values",
  "-\tsession.persist(values)",
  "-\treturn session.store.Save(ctx, session)",
  "+\tsnapshot := session.snapshot()",
  "+\tif err := snapshot.Validate(); err != nil {",
  "+\t\treturn err",
  "+\t}",
  "+\tif err := ctx.Err(); err != nil { return err }",
  "+\tsession.accept(snapshot)",
  "+\treturn session.records.Commit(ctx, snapshot)",
  " }",
];

interface CapturedFile {
  change: WorkspaceFileChange;
  body: readonly string[];
}

function patchFile({ change, body }: CapturedFile) {
  const old = change.previousPath ?? change.path;
  const added = change.status === "added" || change.status === "untracked";
  const deleted = change.status === "deleted";
  const lines = [`diff --git ${JSON.stringify(`a/${old}`)} ${JSON.stringify(`b/${change.path}`)}`];
  if (added) lines.push("new file mode 100644");
  if (deleted) lines.push("deleted file mode 100644");
  if (change.previousPath)
    lines.push(
      `similarity index ${body.length ? 80 : 100}%`,
      `rename from ${change.previousPath}`,
      `rename to ${change.path}`,
    );
  if (change.binary)
    lines.push(
      `Binary files ${added ? "/dev/null" : `a/${old}`} and ${deleted ? "/dev/null" : `b/${change.path}`} differ`,
    );
  else if (body.length)
    lines.push(
      `--- ${added ? "/dev/null" : JSON.stringify(`a/${old}`)}`,
      `+++ ${deleted ? "/dev/null" : JSON.stringify(`b/${change.path}`)}`,
      ...body,
    );
  else if (change.status === "modified") lines.push("old mode 100644", "new mode 100755");
  return [...lines, ""].join("\n");
}

interface ReadSelection {
  firstPath?: string;
  path?: string;
}
const HEAD: VisualRead["baseline"] = {
  type: "head",
  commit: "1234567890abcdef1234567890abcdef12345678",
};
const MERGE_BASE: VisualRead["baseline"] = {
  type: "mergeBase",
  commit: "abcdef1234567890abcdef1234567890abcdef12",
};

function visualRead(
  files: readonly CapturedFile[],
  baseline: VisualRead["baseline"],
  path?: string,
): VisualRead {
  const selected = path ? files.filter((file) => file.change.path === path) : files;
  return {
    type: "visual",
    baseline,
    visual: {
      budget: BUDGET,
      truncated: false,
      manifest: selected.map((file) => file.change),
      patch: selected.map(patchFile).join(""),
    },
  };
}

function dockFile(path: string): CapturedFile {
  return { change: { path, status: "modified", added: 18, removed: 6 }, body: DOCK_LINES };
}

export function reviewDiff({ firstPath = ACTIVE_DIFF_FILE, path }: ReadSelection = {}): VisualRead {
  return visualRead(
    [
      dockFile(firstPath),
      { change: { path: SDK_FILE, status: "modified", added: 7, removed: 3 }, body: SDK_LINES },
    ],
    HEAD,
    path,
  );
}

const RENAME_PATH = "docs/layout/导航说明.md";
const PREVIOUS_PATH = "docs/layout/旧导航.md";
const BINARY_PATH = "assets/flame-logo.png";
export function branchReviewDiff(path?: string): VisualRead {
  return visualRead(
    [
      dockFile(ACTIVE_DIFF_FILE),
      {
        change: {
          path: RENAME_PATH,
          previousPath: PREVIOUS_PATH,
          status: "renamed",
          added: 0,
          removed: 0,
        },
        body: [],
      },
      {
        change: { path: BINARY_PATH, status: "modified", binary: true },
        body: [],
      },
    ],
    MERGE_BASE,
    path,
  );
}
export const BRANCH_REVIEW_DIFF = branchReviewDiff();

export function scaledReview(
  fileCount: number,
  { firstPath, path }: ReadSelection = {},
): VisualRead {
  const files: CapturedFile[] = [];
  const admitted = Math.min(fileCount, BUDGET.files, Math.floor(BUDGET.rows / 25));
  for (let index = 0; index < admitted; index++) {
    const name =
      index === 0 && firstPath
        ? firstPath
        : `src/generated/module-${String(index).padStart(3, "0")}.ts`;
    const lines = ["@@ -1,18 +1,18 @@"];
    for (let line = 1; line <= 12; line++) {
      const code =
        index % 10 === 0
          ? `export const value${line} = ${JSON.stringify("x".repeat(320))};`
          : `export const value${line} = compute(${index}, ${line});`;
      lines.push(` ${code}`);
      if (line % 2 === 0) lines.push(`-${code} // before`, `+${code} // after`);
    }
    files.push({ change: { path: name, status: "modified", added: 6, removed: 6 }, body: lines });
  }
  const result = visualRead(files, HEAD, path);
  return { ...result, visual: { ...result.visual, truncated: admitted < fileCount } };
}

export function evidenceDiff(
  path?: string,
  mode: DiffMode = "worktree",
  refreshed = false,
): VisualRead {
  const files: CapturedFile[] = [
    {
      change: { path: ACTIVE_DIFF_FILE, status: "modified", added: 2, removed: 2 },
      body: [
        "@@ -1,4 +1,4 @@",
        " alpha中🙂omega\r",
        "-old\tvalue\r",
        mode === "base" ? "+branch\tvalue\r" : "+new\tvalue\r",
        " \r",
        "-tail-before",
        "\\ No newline at end of file",
        mode === "base" ? (refreshed ? "+refreshed-after" : "+branch-after") : "+tail-after",
        "\\ No newline at end of file",
      ],
    },
    {
      change: {
        path: "docs/新名字.md",
        previousPath: "docs/旧名字.md",
        status: "renamed",
        added: 0,
        removed: 0,
      },
      body: [],
    },
    { change: { path: "assets/image.bin", status: "modified", binary: true }, body: [] },
  ];
  if (mode === "worktree") {
    files.push({
      change: { path: "empty.txt", status: "untracked", added: 0, removed: 0 },
      body: [],
    });
  }
  return visualRead(files, mode === "base" ? MERGE_BASE : HEAD, path);
}

export function budgetDiff(): VisualRead {
  return {
    type: "visual",
    baseline: HEAD,
    visual: { patch: "", manifest: [], budget: { ...BUDGET, rows: 1 }, truncated: true },
  };
}

export function largeFileDiff(path?: string): VisualRead {
  return visualRead(
    [
      {
        change: { path: ACTIVE_DIFF_FILE, status: "untracked", added: BUDGET.rows - 1, removed: 0 },
        body: [
          `@@ -0,0 +1,${BUDGET.rows - 1} @@`,
          ...Array.from(
            { length: BUDGET.rows - 1 },
            (_, index) => `+export const item${index + 1} = ${index + 1};`,
          ),
        ],
      },
    ],
    HEAD,
    path,
  );
}
