import * as stylex from "@stylexjs/stylex";
import {
  Component,
  useMemo,
  useRef,
  type ContextType,
  type ReactNode,
  type RefObject,
} from "react";
import { presentDiffRows, type PresentedLine } from "./diff-table-lines";
import { PanelLayoutContext } from "./panel-layout";
import { selectAllInReader } from "./reader-selection";
import { ScrollRegion } from "./scroll-region";
import { color, space, surface, type } from "./tokens.stylex";

export type ComparisonRow =
  | { type: "hunk"; text: string }
  | {
      type: "context" | "added" | "deleted";
      code: string;
      leftLine?: number;
      rightLine?: number;
    };

export interface DiffTableLabels {
  region: string;
  beforeLine: string;
  beforeCode: string;
  afterLine: string;
  afterCode: string;
  changedCode: string;
}
const styles = stylex.create({
  table: {
    width: "100%",
    borderCollapse: "collapse",
    fontFamily: "var(--font-mono)",
  },
  cell: {
    paddingBlock: space.s1,
    paddingInline: space.s2,
    textAlign: "start",
    verticalAlign: "top",
    whiteSpace: "pre",
  },
  number: {
    textAlign: "end",
    fontVariantNumeric: "tabular-nums",
    color: color.fgMuted,
    userSelect: "none",
  },
  marker: {
    display: "inline-block",
    inlineSize: "2ch",
    textAlign: "start",
  },
  added: { backgroundColor: surface.successWash },
  deleted: { backgroundColor: surface.negativeWash },
  hunk: { backgroundColor: surface.sunken, color: color.fgMuted },
  change: {
    color: "inherit",
    backgroundColor: "transparent",
    borderBlockEndWidth: 1,
    borderBlockEndStyle: "solid",
    borderBlockEndColor: "currentColor",
  },
});
function CodeCell({ line }: { line?: PresentedLine }) {
  const kind = line?.source.type === "context" ? undefined : line?.source.type;
  return (
    <td
      data-diff-line={line?.sourceIndex}
      {...stylex.props(styles.cell, kind && styles[kind])}
    >
      <span aria-hidden="true" {...stylex.props(styles.number, styles.marker)}>
        {kind === "added" ? "+" : kind === "deleted" ? "−" : ""}
      </span>
      <code>
        {line?.parts
          ? line.parts.map((part, index) =>
              part.changed ? (
                <mark key={index} {...stylex.props(styles.change)}>
                  {part.text}
                </mark>
              ) : (
                part.text
              ),
            )
          : line?.source.code}
      </code>
    </td>
  );
}

interface ReadingViewport {
  top: number;
  bottom: number;
  left: number;
  right: number;
  scroller: Element | null;
}
interface ReadingAnchor {
  line: string;
  offset: number;
  scroller: Element;
}
function readingViewport(
  body: HTMLElement,
  controls: Element | null,
): ReadingViewport {
  const document = body.ownerDocument;
  const viewport: ReadingViewport = {
    top: 0,
    bottom: document.defaultView?.innerHeight ?? 0,
    left: 0,
    right: document.defaultView?.innerWidth ?? 0,
    scroller: null,
  };
  for (let parent = body.parentElement; parent; parent = parent.parentElement) {
    const style = getComputedStyle(parent);
    const bounds = parent.getBoundingClientRect();
    if (style.overflowX && style.overflowX !== "visible") {
      viewport.left = Math.max(viewport.left, bounds.left);
      viewport.right = Math.min(viewport.right, bounds.right);
    }
    if (style.overflowY && style.overflowY !== "visible") {
      viewport.top = Math.max(viewport.top, bounds.top);
      viewport.bottom = Math.min(viewport.bottom, bounds.bottom);
    }
    if (
      !viewport.scroller &&
      /^(auto|scroll|overlay)$/.test(style.overflowY) &&
      parent.scrollHeight > parent.clientHeight
    ) {
      viewport.scroller = parent;
    }
  }
  const root = document.scrollingElement;
  if (!viewport.scroller && root && root.scrollHeight > root.clientHeight)
    viewport.scroller = root;
  if (controls)
    viewport.top = Math.max(
      viewport.top,
      controls.getBoundingClientRect().bottom,
    );
  return viewport;
}
interface DiffRowsProps {
  source: readonly ComparisonRow[];
  layout: "unified" | "split";
  bodyRef: RefObject<HTMLTableSectionElement | null>;
  children: ReactNode;
}

// An effect sees the replacement rows; the reading anchor needs the pre-mutation DOM.
class DiffRows extends Component<
  DiffRowsProps,
  Record<string, never>,
  ReadingAnchor | null
> {
  static contextType = PanelLayoutContext;
  declare context: ContextType<typeof PanelLayoutContext>;

  getSnapshotBeforeUpdate(previous: DiffRowsProps): ReadingAnchor | null {
    if (
      previous.layout === this.props.layout ||
      previous.source !== this.props.source
    )
      return null;
    const body = this.props.bodyRef.current;
    if (
      !body ||
      body.closest("[hidden], [inert]") ||
      getComputedStyle(body).visibility === "hidden"
    )
      return null;
    const viewport = readingViewport(
      body,
      this.context?.controlsRef?.current ?? null,
    );
    if (
      !viewport.scroller ||
      viewport.top >= viewport.bottom ||
      viewport.left >= viewport.right
    )
      return null;
    for (const cell of body.querySelectorAll<HTMLElement>("[data-diff-line]")) {
      const bounds = cell.getBoundingClientRect();
      if (
        bounds.bottom > viewport.top &&
        bounds.top < viewport.bottom &&
        bounds.right > viewport.left &&
        bounds.left < viewport.right
      ) {
        return {
          line: cell.dataset.diffLine!,
          offset: bounds.top - viewport.top,
          scroller: viewport.scroller,
        };
      }
    }
    return null;
  }

  componentDidUpdate(
    _previous: DiffRowsProps,
    _state: unknown,
    anchor?: ReadingAnchor | null,
  ): void {
    const body = this.props.bodyRef.current;
    if (
      !anchor ||
      !body ||
      !anchor.scroller.isConnected ||
      !anchor.scroller.contains(body)
    )
      return;
    const cell = body.querySelector<HTMLElement>(
      `[data-diff-line="${anchor.line}"]`,
    );
    if (!cell) return;
    const viewport = readingViewport(
      body,
      this.context?.controlsRef?.current ?? null,
    );
    if (viewport.scroller !== anchor.scroller) return;
    anchor.scroller.scrollTop +=
      cell.getBoundingClientRect().top - viewport.top - anchor.offset;
  }

  render() {
    return <tbody ref={this.props.bodyRef}>{this.props.children}</tbody>;
  }
}

export function DiffTable({
  rows,
  layout,
  label,
  labels,
}: {
  rows: readonly ComparisonRow[];
  layout: "unified" | "split";
  label: string;
  labels: DiffTableLabels;
}) {
  const split = layout === "split";
  const body = useRef<HTMLTableSectionElement>(null);
  const presented = useMemo(() => presentDiffRows(rows, split), [rows, split]);
  return (
    <ScrollRegion
      label={labels.region}
      onKeyDown={(event) => {
        if (body.current) selectAllInReader(event, body.current);
      }}
    >
      <table data-layout={layout} {...stylex.props(styles.table, type.code)}>
        <caption className="sr-only">{label}</caption>
        <thead className="sr-only">
          <tr>
            {(split
              ? [
                  labels.beforeLine,
                  labels.beforeCode,
                  labels.afterLine,
                  labels.afterCode,
                ]
              : [labels.beforeLine, labels.afterLine, labels.changedCode]
            ).map((title, index) => (
              <th key={index} scope="col">
                {title}
              </th>
            ))}
          </tr>
        </thead>
        <DiffRows bodyRef={body} source={rows} layout={layout}>
          {presented.map((row) =>
            row.type === "hunk" ? (
              <tr key={row.key}>
                <td
                  colSpan={split ? 4 : 3}
                  {...stylex.props(styles.cell, styles.hunk)}
                >
                  {row.text}
                </td>
              </tr>
            ) : split ? (
              <tr key={row.key}>
                <td
                  {...stylex.props(
                    styles.cell,
                    styles.number,
                    row.before?.source.type === "deleted" && styles.deleted,
                  )}
                >
                  {row.before?.source.leftLine}
                </td>
                <CodeCell line={row.before} />
                <td
                  {...stylex.props(
                    styles.cell,
                    styles.number,
                    row.after?.source.type === "added" && styles.added,
                  )}
                >
                  {row.after?.source.rightLine}
                </td>
                <CodeCell line={row.after} />
              </tr>
            ) : (
              <tr key={row.key}>
                <td {...stylex.props(styles.cell, styles.number)}>
                  {row.before?.source.leftLine}
                </td>
                <td {...stylex.props(styles.cell, styles.number)}>
                  {row.after?.source.rightLine}
                </td>
                <CodeCell line={row.before ?? row.after} />
              </tr>
            ),
          )}
        </DiffRows>
      </table>
    </ScrollRegion>
  );
}
