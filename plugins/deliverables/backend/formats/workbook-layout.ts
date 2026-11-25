import { cellValueSchema, type CellValue } from "../../contracts/content";
import { type WorkbookLayout } from "../../contracts/layouts";
import { OfficePackage } from "./archive";
import { worksheetCells } from "./workbook";
import { protectedRegions } from "./workbook-editing";
import {
  addressName,
  cellAddress,
  cellDescription,
  cellValue,
  isSheetElement,
  sharedStrings,
  workbookSheets,
} from "./workbook-cells";
import type { XmlNode } from "./xml";

// A setCell value reproduces the cell only when the edit schema can carry it;
// shared and array formula members, error literals and native decimals cannot.
function input(
  cell: XmlNode,
  shown: ReturnType<typeof cellValue>,
): CellValue | undefined {
  const formula = cell.children.find((node) => isSheetElement(node, "f"));
  const candidate = formula
    ? formula.text && (formula.attributes.t ?? "normal") === "normal"
      ? { type: "formula", value: `=${formula.text}` }
      : undefined
    : shown.type === "error" || "representation" in shown
      ? undefined
      : shown;
  const parsed = cellValueSchema.safeParse(candidate);
  return parsed.success ? parsed.data : undefined;
}

function frozen(pane: XmlNode | undefined) {
  if (!pane || !["frozen", "frozenSplit"].includes(pane.attributes.state ?? ""))
    return { rows: 0, columns: 0 };
  const split = (value: string | undefined) =>
    value && /^[0-9]+$/u.test(value) ? Number(value) : 0;
  return {
    rows: split(pane.attributes.ySplit),
    columns: split(pane.attributes.xSplit),
  };
}

export function workbookLayout(archive: OfficePackage): WorkbookLayout {
  const strings = sharedStrings(archive);
  return {
    sheets: workbookSheets(archive).map(({ name, part }) => {
      const document = archive.xml(part);
      const cells = worksheetCells(document);
      const locked = protectedRegions(archive, part, document, cells);
      const projected = [...cells].map(([ref, cell]) => {
        const shown = cellValue(cell, strings);
        const value = input(cell, shown);
        return {
          ref,
          expect: cellDescription(cell, strings),
          shown,
          ...(value ? { input: value } : {}),
        };
      });
      const corners = [
        ...projected.map(({ ref }) => cellAddress(ref)),
        ...locked.map(({ range }) => range.end),
      ];
      return {
        id: part,
        name,
        rows: corners.reduce((most, point) => Math.max(most, point.row), 0),
        columns: corners.reduce(
          (most, point) => Math.max(most, point.column),
          0,
        ),
        frozen: frozen(
          document.nodes.find((node) => isSheetElement(node, "pane")),
        ),
        locked: locked.map(({ range, reason, anchor }) => ({
          range: `${addressName(range.start.column, range.start.row)}:${addressName(range.end.column, range.end.row)}`,
          reason,
          ...(anchor ? { anchor: addressName(anchor.column, anchor.row) } : {}),
        })),
        cells: projected,
      };
    }),
  };
}
