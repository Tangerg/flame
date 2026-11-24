// @pierre/diffs renders every source row as [data-line] inside a [data-code]
// column. A file wraps the column in [data-file] and its selectable text in
// [data-content]; a diff stamps rows with their [data-line-index] pair.
// Flame reads that shadow DOM only through these queries.

export function codeLines(root: ParentNode): NodeListOf<HTMLElement> {
  return root.querySelectorAll<HTMLElement>("[data-code] [data-line]");
}

export function fileCodeColumn(root: ParentNode): HTMLElement | null {
  return root.querySelector<HTMLElement>("[data-file] > [data-code]");
}

export function columnContent(column: Element): Element | null {
  return column.querySelector(":scope > [data-content]");
}

// The rows of a file's selectable content, or null for any other element.
export function fileContentRows(element: Element): Element[] | null {
  if (!element.matches("[data-file] > [data-code] > [data-content]")) return null;
  return [...element.querySelectorAll(":scope > [data-line]")];
}

export function gutterNumberLabels(column: Element): NodeListOf<Element> {
  return column.querySelectorAll("[data-gutter] [data-column-number] > [data-line-number-content]");
}

export function diffColumns(root: ParentNode): NodeListOf<Element> {
  return root.querySelectorAll("[data-code]");
}

export function indexedLines(column: Element): NodeListOf<Element> {
  return column.querySelectorAll("[data-line][data-line-index]");
}

export function indexedLine(root: ParentNode, index: string): Element | null {
  return root.querySelector(`[data-code] [data-line][data-line-index="${CSS.escape(index)}"]`);
}

// Pierre numbers rows from 1 within the rendered text, not the source file.
export function rowNumber(line: Element): number {
  return Number(line.getAttribute("data-line"));
}

export function rowIndexPair(line: Element): string {
  return line.getAttribute("data-line-index") ?? "";
}

export function isDeletionRow(line: Element): boolean {
  return (
    line.closest("code[data-deletions]") !== null ||
    line.getAttribute("data-line-type")?.endsWith("deletion") === true
  );
}

export function gutterRowNumber(label: Element): number {
  return Number(label.parentElement?.getAttribute("data-column-number"));
}
