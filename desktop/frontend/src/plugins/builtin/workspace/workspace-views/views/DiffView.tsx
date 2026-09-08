import * as stylex from "@stylexjs/stylex";
import type { Highlighter } from "shiki";
import type { WorkspaceDiffRow } from "@/plugins/builtin/workspace/application/workspaceQueries";
import { useMemo } from "react";
import { intraLineDiff } from "../intraLineDiff";
import { stripCodeWrapper, useCodeHighlighter } from "@/lib/highlight/useCodeHighlight";
import { langFromPath, resolveLang } from "@/lib/highlight/shiki";
import { color, type as typeStep } from "@/styles/tokens.stylex";
import { codeStyles as cs } from "./viewStyles";

export type DiffLayout = "unified" | "split";

function keyFor(row: WorkspaceDiffRow, i: number): string {
  if (row.type === "hunk") return `h:${i}:${row.text}`;
  if (row.type === "added") return `+:${row.rightLine}`;
  if (row.type === "deleted") return `-:${row.leftLine}`;
  return `=:${row.leftLine}-${row.rightLine}`;
}

/** What a diff row type looks like, and what it reads as. Context has no tint: an unchanged
 *  line is the ground the other two are read against. */
const ROW_STYLE = {
  added: { tone: cs.rowAdded, meta: cs.metaAdded, sign: "+" },
  deleted: { tone: cs.rowDeleted, meta: cs.metaDeleted, sign: "−" },
  context: { tone: null, meta: cs.metaContext, sign: " " },
  // The keys are the contract; `unknown` for the styles because `satisfies` checks excess
  // properties, and what StyleX hands back is not worth restating here.
} as const satisfies Record<
  "added" | "deleted" | "context",
  { sign: string; tone: unknown; meta: unknown }
>;

const wordMark = (ink: string) =>
  `text-decoration-line:underline;text-decoration-color:${ink};text-decoration-thickness:2px;text-underline-offset:2px;text-decoration-skip-ink:none`;
const WD_DEL_STYLE = wordMark("var(--color-diff-deleted-meta)");
const WD_ADD_STYLE = wordMark("var(--color-diff-added-meta)");

type WordDecoration = { start: number; end: number; properties: { style: string } };

function highlightInline(
  h: Highlighter,
  code: string,
  theme: string,
  lang: string,
  decorations: WordDecoration[],
): string {
  return stripCodeWrapper(h.codeToHtml(code || " ", { lang, theme, decorations }), code);
}

function computeWordRanges(rows: WorkspaceDiffRow[]): Map<WorkspaceDiffRow, [number, number]> {
  const ranges = new Map<WorkspaceDiffRow, [number, number]>();
  let dels: Extract<WorkspaceDiffRow, { type: "deleted" }>[] = [];
  let adds: Extract<WorkspaceDiffRow, { type: "added" }>[] = [];
  const flush = () => {
    const n = Math.min(dels.length, adds.length);
    for (let i = 0; i < n; i++) {
      const { del, add } = intraLineDiff(dels[i]!.code, adds[i]!.code);
      if (del) ranges.set(dels[i]!, del);
      if (add) ranges.set(adds[i]!, add);
    }
    dels = [];
    adds = [];
  };
  for (const row of rows) {
    if (row.type === "deleted") dels.push(row);
    else if (row.type === "added") adds.push(row);
    else flush();
  }
  flush();
  return ranges;
}

const dv = stylex.create({
  /** A diff line wraps rather than scrolls, and breaks mid-token when a token is longer than
   *  the pane — a minified line or a base64 blob otherwise widens the whole table. */
  cell: { minWidth: 0, whiteSpace: "pre-wrap", overflowWrap: "anywhere" },
  /** Unhighlighted code is the fallback, and reads a step back so the wait is legible. */
  plain: { color: color.fgSoft },
});

function CodeCell({ code, html }: { code: string; html: string | undefined }) {
  return html ? (
    <span {...stylex.props(dv.cell)} dangerouslySetInnerHTML={{ __html: html }} />
  ) : (
    <span {...stylex.props(dv.cell, dv.plain)}>{code}</span>
  );
}

export function DiffView({
  rows,
  layout = "unified",
  path,
}: {
  rows: WorkspaceDiffRow[];
  layout?: DiffLayout;
  path?: string;
}) {
  const { highlighter, theme: shikiTheme } = useCodeHighlighter();

  const wordRanges = useMemo(() => computeWordRanges(rows), [rows]);
  const highlighted = useMemo(() => {
    if (!highlighter) return null;
    const lang = resolveLang(highlighter, path ? langFromPath(path) : "text");
    const out = new Map<WorkspaceDiffRow, string>();
    for (const row of rows) {
      if (row.type === "hunk") continue;
      const range = wordRanges.get(row);
      const style =
        row.type === "deleted" ? WD_DEL_STYLE : row.type === "added" ? WD_ADD_STYLE : "";
      const decorations: WordDecoration[] =
        range && style ? [{ start: range[0], end: range[1], properties: { style } }] : [];
      out.set(row, highlightInline(highlighter, row.code, shikiTheme, lang, decorations));
    }
    return out;
  }, [highlighter, rows, shikiTheme, wordRanges, path]);

  if (layout === "split") {
    return <SplitDiff rows={rows} highlighted={highlighted} />;
  }

  return (
    <div {...stylex.props(cs.sheet, typeStep.code)}>
      {rows.map((row, i) => {
        const k = keyFor(row, i);
        if (row.type === "hunk") return <HunkRow key={k} text={row.text} />;
        const style = ROW_STYLE[row.type];
        const lnum = row.type === "deleted" ? row.leftLine : row.rightLine;
        return (
          <div key={k} {...stylex.props(cs.lineRow, cs.gutterPair, style.tone)}>
            <span {...stylex.props(cs.lineMeta, style.meta, typeStep.uiSm)}>{lnum}</span>
            <span {...stylex.props(cs.signMeta, style.meta, typeStep.uiSm)}>{style.sign}</span>
            <CodeCell code={row.code} html={highlighted?.get(row)} />
          </div>
        );
      })}
    </div>
  );
}

function HunkRow({ text }: { text: string }) {
  return <div {...stylex.props(cs.hunk, typeStep.uiSm)}>{text}</div>;
}

type Half = Extract<WorkspaceDiffRow, { type: "context" | "deleted" | "added" }> | null;
interface SplitRow {
  left: Half;
  right: Half;
}

function toSplitRows(rows: WorkspaceDiffRow[]): ({ hunk: string } | SplitRow)[] {
  const out: ({ hunk: string } | SplitRow)[] = [];
  let dels: Extract<WorkspaceDiffRow, { type: "deleted" }>[] = [];
  let adds: Extract<WorkspaceDiffRow, { type: "added" }>[] = [];
  const flush = () => {
    const n = Math.max(dels.length, adds.length);
    for (let i = 0; i < n; i++) out.push({ left: dels[i] ?? null, right: adds[i] ?? null });
    dels = [];
    adds = [];
  };
  for (const row of rows) {
    if (row.type === "hunk") {
      flush();
      out.push({ hunk: row.text });
    } else if (row.type === "context") {
      flush();
      out.push({ left: row, right: row });
    } else if (row.type === "deleted") {
      dels.push(row);
    } else {
      adds.push(row);
    }
  }
  flush();
  return out;
}

function SplitDiff({
  rows,
  highlighted,
}: {
  rows: WorkspaceDiffRow[];
  highlighted: Map<WorkspaceDiffRow, string> | null;
}) {
  const split = useMemo(() => toSplitRows(rows), [rows]);
  return (
    <div {...stylex.props(cs.sheet, typeStep.code)}>
      {split.map((row, i) => {
        if ("hunk" in row) return <HunkRow key={`h:${i}`} text={row.hunk} />;
        return (
          <div key={`s:${i}`} {...stylex.props(cs.sideBySide)}>
            <DiffSide row={row.left} side="left" highlighted={highlighted} />
            <DiffSide row={row.right} side="right" highlighted={highlighted} />
          </div>
        );
      })}
    </div>
  );
}

function DiffSide({
  row,
  side,
  highlighted,
}: {
  row: Half;
  side: "left" | "right";
  highlighted: Map<WorkspaceDiffRow, string> | null;
}) {
  if (!row) return <div {...stylex.props(cs.blank)} />;
  const style = ROW_STYLE[row.type];
  const lnum =
    row.type === "deleted"
      ? row.leftLine
      : row.type === "added"
        ? row.rightLine
        : side === "left"
          ? row.leftLine
          : row.rightLine;
  const sign = row.type === "context" ? "" : style.sign;
  return (
    <div {...stylex.props(cs.lineRow, cs.gutterSign, style.tone)}>
      <span {...stylex.props(cs.lineMeta, style.meta, typeStep.uiSm)}>{lnum}</span>
      <span {...stylex.props(cs.signMeta, style.meta, typeStep.uiSm)}>{sign}</span>
      <CodeCell code={row.code} html={highlighted?.get(row)} />
    </div>
  );
}
