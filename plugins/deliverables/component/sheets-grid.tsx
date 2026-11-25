import {
  useLayoutEffect,
  useMemo,
  useRef,
  useState,
  type KeyboardEvent,
  type UIEvent,
} from "react";
import type { Translate } from "@flame/component-sdk";
import { type CellValue } from "../contracts/content";
import {
  type SheetCellView,
  type SheetView,
  type ShownCell,
} from "../contracts/layouts";
import { entryText } from "./sheets-draft";

const ROW_HEIGHT = 28;
const VIEW_ROWS = 18;
const OVERSCAN = 8;

export interface Point {
  row: number;
  column: number;
}

// Typing starts in the cell or in the contents field; only the cell that owns
// the entry renders an input, so the two never compete for focus.
export interface Entry {
  ref: string;
  text: string;
  source: "cell" | "bar";
}

function columnName(column: number): string {
  let name = "";
  for (let rest = column; rest > 0; rest = Math.floor((rest - 1) / 26))
    name = String.fromCharCode(65 + ((rest - 1) % 26)) + name;
  return name;
}

export const refOf = ({ row, column }: Point) => `${columnName(column)}${row}`;

export function pointOf(ref: string): Point {
  const match = /^([A-Z]+)([0-9]+)$/u.exec(ref)!;
  let column = 0;
  for (const letter of match[1]!)
    column = column * 26 + letter.charCodeAt(0) - 64;
  return { row: Number(match[2]), column };
}

function rangeOf(ref: string) {
  const [start, end = start] = ref.split(":");
  return { start: pointOf(start!), end: pointOf(end!) };
}

export function bounds(anchor: Point, focus: Point) {
  return {
    start: {
      row: Math.min(anchor.row, focus.row),
      column: Math.min(anchor.column, focus.column),
    },
    end: {
      row: Math.max(anchor.row, focus.row),
      column: Math.max(anchor.column, focus.column),
    },
  };
}

export const within = (
  range: { start: Point; end: Point },
  point: Point,
): boolean =>
  point.row >= range.start.row &&
  point.row <= range.end.row &&
  point.column >= range.start.column &&
  point.column <= range.end.column;

export function lockOf(sheet: SheetView, point: Point) {
  return sheet.locked.find(
    (entry) =>
      within(rangeOf(entry.range), point) &&
      !(entry.anchor && entry.anchor === refOf(point)),
  )?.reason;
}

export function shownText(shown: ShownCell): string {
  switch (shown.type) {
    case "blank":
      return "";
    case "boolean":
      return shown.value ? "TRUE" : "FALSE";
    default:
      return String(shown.value);
  }
}

// A formula shows its last calculated result; edited or uncalculated formulas
// show their text until recalculation produces one.
function display(
  saved: SheetCellView | undefined,
  edited: CellValue | undefined,
): { text: string; pending: boolean; numeric: boolean } {
  if (edited)
    return edited.type === "formula"
      ? { text: edited.value, pending: true, numeric: false }
      : {
          text: entryText(edited).replace(/^'/u, ""),
          pending: false,
          numeric: edited.type === "number",
        };
  if (!saved) return { text: "", pending: false, numeric: false };
  if (saved.input?.type === "formula" && saved.shown.type === "blank")
    return { text: saved.input.value, pending: true, numeric: false };
  return {
    text: shownText(saved.shown),
    pending: false,
    numeric: saved.shown.type === "number",
  };
}

const STEPS: Record<string, [number, number]> = {
  ArrowUp: [-1, 0],
  ArrowDown: [1, 0],
  ArrowLeft: [0, -1],
  ArrowRight: [0, 1],
};

type GridIntent =
  | { type: "move"; row: number; column: number; extend: boolean }
  | { type: "edit"; text?: string }
  | { type: "clear" }
  | { type: "undo" };

// Arrows move (Shift extends), Tab moves across, Enter or F2 edits the cell
// and a printable key replaces it.
function gridIntent(event: KeyboardEvent<HTMLElement>): GridIntent | undefined {
  const command = event.metaKey || event.ctrlKey;
  if (command && event.key.toLowerCase() === "z") return { type: "undo" };
  if (command || event.altKey) return undefined;
  if (event.key === "Tab")
    return {
      type: "move",
      row: 0,
      column: event.shiftKey ? -1 : 1,
      extend: false,
    };
  const step = STEPS[event.key];
  if (step)
    return {
      type: "move",
      row: step[0],
      column: step[1],
      extend: event.shiftKey,
    };
  if (event.key === "Enter" || event.key === "F2") return { type: "edit" };
  if (event.key === "Delete" || event.key === "Backspace")
    return { type: "clear" };
  if (event.key.length === 1) return { type: "edit", text: event.key };
  return undefined;
}

export function Grid({
  sheet,
  edits,
  anchor,
  focus,
  editing,
  editable,
  onSelect,
  onEdit,
  onCommit,
  onClear,
  onUndo,
  t,
}: {
  sheet: SheetView;
  edits: ReadonlyMap<string, CellValue>;
  anchor: Point;
  focus: Point;
  editing: Entry | undefined;
  editable: boolean;
  onSelect(focus: Point, extend: boolean): void;
  onEdit(editing: Entry | undefined): void;
  onCommit(move?: Point): void;
  onClear(): void;
  onUndo(): void;
  t: Translate;
}) {
  const [scrollTop, setScrollTop] = useState(0);
  const saved = useMemo(
    () => new Map(sheet.cells.map((cell) => [cell.ref, cell])),
    [sheet.cells],
  );
  const rows = Math.max(sheet.rows + 20, 40, focus.row + 1);
  const columns = Math.max(sheet.columns + 3, 8, focus.column + 1);
  const first = Math.max(1, Math.floor(scrollTop / ROW_HEIGHT) - OVERSCAN);
  const last = Math.min(rows, first + VIEW_ROWS + OVERSCAN * 2);
  const range = bounds(anchor, focus);
  const columnList = Array.from({ length: columns }, (_, index) => index + 1);
  const grid = useRef<HTMLDivElement>(null);
  const focusedCell = useRef<HTMLTableCellElement>(null);
  // Rows are windowed, so vertical reveal is computed in move; columns are
  // all rendered with content widths, so the focused cell is measured here.
  useLayoutEffect(() => {
    const viewport = grid.current;
    const cell = focusedCell.current;
    if (!viewport || !cell) return;
    const header = viewport.querySelector("th")?.offsetWidth ?? 0;
    if (cell.offsetLeft - header < viewport.scrollLeft)
      viewport.scrollLeft = cell.offsetLeft - header;
    else if (
      cell.offsetLeft + cell.offsetWidth >
      viewport.scrollLeft + viewport.clientWidth
    )
      viewport.scrollLeft =
        cell.offsetLeft + cell.offsetWidth - viewport.clientWidth;
  }, [focus.column]);
  const writable = (point: Point) => editable && !lockOf(sheet, point);

  const move = (point: Point, extend = false) => {
    const next = {
      row: Math.min(Math.max(point.row, 1), 1048576),
      column: Math.min(Math.max(point.column, 1), 16384),
    };
    onSelect(next, extend);
    const viewport = grid.current;
    if (!viewport) return;
    const top = (next.row - 1) * ROW_HEIGHT;
    if (top < viewport.scrollTop) viewport.scrollTop = top;
    else if (top + ROW_HEIGHT * 2 > viewport.scrollTop + viewport.clientHeight)
      viewport.scrollTop = top + ROW_HEIGHT * 2 - viewport.clientHeight;
  };
  const begin = (text: string) => {
    if (!writable(focus)) return;
    onEdit({ ref: refOf(focus), text, source: "cell" });
  };
  const keys = (event: KeyboardEvent<HTMLDivElement>) => {
    if (editing) return;
    const intent = gridIntent(event);
    if (!intent) return;
    event.preventDefault();
    switch (intent.type) {
      case "undo":
        return onUndo();
      case "clear":
        return onClear();
      case "move":
        return move(
          { row: focus.row + intent.row, column: focus.column + intent.column },
          intent.extend,
        );
      case "edit":
        return begin(
          intent.text ??
            entryText(
              edits.get(refOf(focus)) ?? saved.get(refOf(focus))?.input,
            ),
        );
    }
  };
  const editorKeys = (event: KeyboardEvent<HTMLInputElement>) => {
    if (!editing) return;
    if (event.key === "Escape") {
      event.preventDefault();
      onEdit(undefined);
      grid.current?.focus({ preventScroll: true });
    } else if (event.key === "Enter" || event.key === "Tab") {
      event.preventDefault();
      const next =
        event.key === "Enter"
          ? { row: focus.row + (event.shiftKey ? -1 : 1), column: focus.column }
          : {
              row: focus.row,
              column: focus.column + (event.shiftKey ? -1 : 1),
            };
      onCommit(next);
      grid.current?.focus({ preventScroll: true });
    }
  };
  const cellStyle = {
    height: ROW_HEIGHT,
    boxSizing: "border-box" as const,
    borderRight: "1px solid var(--color-border)",
    borderBottom: "1px solid var(--color-border)",
    padding: "0 6px",
    whiteSpace: "nowrap" as const,
    overflow: "hidden",
    textOverflow: "ellipsis",
    maxWidth: "16rem",
    minWidth: "5.5rem",
  };
  const headerStyle = {
    ...cellStyle,
    position: "sticky" as const,
    background: "var(--color-surface-2)",
    color: "var(--color-text-soft)",
    fontWeight: 500,
    textAlign: "center" as const,
    minWidth: "2.75rem",
  };
  return (
    <div
      ref={grid}
      role="grid"
      aria-label={t("sheets.grid", { name: sheet.name })}
      aria-rowcount={rows}
      aria-colcount={columns}
      tabIndex={0}
      onKeyDown={keys}
      onScroll={(event: UIEvent<HTMLDivElement>) =>
        setScrollTop(event.currentTarget.scrollTop)
      }
      style={{
        maxHeight: ROW_HEIGHT * (VIEW_ROWS + 1),
        overflow: "auto",
        border: "1px solid var(--color-border)",
        borderRadius: "var(--shape-sm)",
        background: "var(--color-surface)",
        fontVariantNumeric: "tabular-nums",
        userSelect: "none",
      }}
    >
      <table
        style={{
          borderCollapse: "separate",
          borderSpacing: 0,
          tableLayout: "auto",
        }}
      >
        <thead>
          <tr role="row">
            <th
              style={{ ...headerStyle, top: 0, left: 0, zIndex: 3 }}
              aria-hidden
            />
            {columnList.map((column) => (
              <th
                key={column}
                role="columnheader"
                style={{ ...headerStyle, top: 0, zIndex: 2 }}
              >
                {columnName(column)}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {first > 1 && (
            <tr aria-hidden style={{ height: (first - 1) * ROW_HEIGHT }} />
          )}
          {Array.from(
            { length: last - first + 1 },
            (_, index) => first + index,
          ).map((row) => (
            <tr key={row} role="row" aria-rowindex={row + 1}>
              <th
                role="rowheader"
                style={{ ...headerStyle, left: 0, zIndex: 1 }}
              >
                {row}
              </th>
              {columnList.map((column) => {
                const point = { row, column };
                const ref = refOf(point);
                const edited = edits.get(ref);
                const shown = display(saved.get(ref), edited);
                const lock = lockOf(sheet, point);
                const focused = row === focus.row && column === focus.column;
                const selected = within(range, point);
                return (
                  <td
                    key={column}
                    ref={focused ? focusedCell : undefined}
                    role="gridcell"
                    aria-selected={selected}
                    aria-readonly={lock ? true : undefined}
                    title={lock ? t(`sheets.locked.${lock}`) : undefined}
                    onPointerDown={(event) => {
                      if (editing?.ref === ref && editing.source === "cell")
                        return;
                      move(point, event.shiftKey);
                    }}
                    onDoubleClick={() => {
                      if (writable(point))
                        onEdit({
                          ref,
                          text: entryText(edited ?? saved.get(ref)?.input),
                          source: "cell",
                        });
                    }}
                    style={{
                      ...cellStyle,
                      textAlign: shown.numeric ? "end" : "start",
                      color: shown.pending
                        ? "var(--color-text-soft)"
                        : undefined,
                      background: edited
                        ? "var(--color-info-wash)"
                        : lock
                          ? "var(--color-sunken)"
                          : selected && !focused
                            ? "var(--color-surface-2)"
                            : undefined,
                      outline: focused
                        ? "2px solid var(--color-accent)"
                        : undefined,
                      outlineOffset: -2,
                    }}
                  >
                    {editing?.ref === ref && editing.source === "cell" ? (
                      <input
                        autoFocus
                        aria-label={t("sheets.formula", { ref })}
                        value={editing.text}
                        onFocus={(event) =>
                          event.currentTarget.setSelectionRange(
                            editing.text.length,
                            editing.text.length,
                          )
                        }
                        onChange={(event) =>
                          onEdit({
                            ref,
                            text: event.target.value,
                            source: "cell",
                          })
                        }
                        onKeyDown={editorKeys}
                        onBlur={() => onCommit()}
                        style={{
                          width: "100%",
                          border: 0,
                          outline: "none",
                          background: "transparent",
                          font: "inherit",
                          color: "inherit",
                          padding: 0,
                        }}
                      />
                    ) : editing?.ref === ref ? (
                      editing.text
                    ) : (
                      shown.text
                    )}
                  </td>
                );
              })}
            </tr>
          ))}
          {last < rows && (
            <tr aria-hidden style={{ height: (rows - last) * ROW_HEIGHT }} />
          )}
        </tbody>
      </table>
    </div>
  );
}
