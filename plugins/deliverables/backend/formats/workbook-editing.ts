import type { CellValue, EditorOperation } from "../../contracts/content";
import {
  OfficePackage,
  relationshipTarget,
  relationshipsPath,
  CONTENT_TYPES_PART,
  MAIN_PARTS,
} from "./archive";
import {
  cellAddress,
  cellDescription,
  cellRange,
  isSheetElement,
  rangeContains,
  sharedStrings,
  workbookSheets,
  xlsxBounds,
  type CellAddress,
  type CellRange,
} from "./workbook-cells";
import {
  attribute,
  escapeXML,
  namespaces,
  openTag,
  patchXML,
  tagName,
  type XmlDocument,
  type XmlNode,
  type XmlPatch,
} from "./xml";
import { cellMarkup, worksheetCells } from "./workbook";

function clearCalculation(archive: OfficePackage): void {
  for (const sheet of workbookSheets(archive)) {
    const document = archive.xml(sheet.part);
    const patches: XmlPatch[] = [];
    for (const cell of worksheetCells(document).values()) {
      if (!cell.children.some((node) => isSheetElement(node, "f"))) continue;
      for (const value of cell.children.filter((node) =>
        isSheetElement(node, "v"),
      ))
        patches.push({ start: value.start, end: value.end, value: "" });
    }
    if (patches.length)
      archive.setXML(sheet.part, patchXML(document.source, patches));
  }
  for (const part of archive.parts.keys()) {
    if (!/^xl\/charts\/.*\.xml$/u.test(part)) continue;
    const document = archive.xml(part);
    const caches = document.nodes.filter(
      (node) =>
        node.uri === namespaces.chart &&
        ["numCache", "strCache"].includes(node.local),
    );
    if (caches.length)
      archive.setXML(
        part,
        patchXML(
          document.source,
          caches.map((node) => ({
            start: node.start,
            end: node.end,
            value: "",
          })),
        ),
      );
  }
  const workbook = archive.xml(MAIN_PARTS.xlsx);
  const calc = workbook.root.children.find((node) =>
    isSheetElement(node, "calcPr"),
  );
  const name = tagName(workbook.root, "calcPr");
  const attributes = {
    ...calc?.attributes,
    calcMode: "auto",
    fullCalcOnLoad: "1",
    forceFullCalc: "1",
    calcId: "0",
  };
  const value = `<${name}${Object.entries(attributes)
    .map(([key, value]) => ` ${key}="${escapeXML(value)}"`)
    .join("")}/>`;
  const before = workbook.root.children.find((node) =>
    [
      "oleSize",
      "customWorkbookViews",
      "pivotCaches",
      "smartTagPr",
      "smartTagTypes",
      "webPublishing",
      "fileRecoveryPr",
      "webPublishObjects",
      "extLst",
    ].includes(node.local),
  );
  const point = before?.start ?? workbook.root.closeStart;
  archive.setXML(
    MAIN_PARTS.xlsx,
    patchXML(workbook.source, [
      { start: calc?.start ?? point, end: calc?.end ?? point, value },
    ]),
  );
  const deleted = new Set<string>();
  const relationships = archive.xml("xl/_rels/workbook.xml.rels");
  const relations = relationships.root.children.filter((node) =>
    node.attributes.Type?.endsWith("/calcChain"),
  );
  for (const relation of relations) {
    const path = relationshipTarget(
      MAIN_PARTS.xlsx,
      relation.attributes.Target!,
    );
    archive.parts.delete(path);
    deleted.add(`/${path}`);
  }
  if (relations.length)
    archive.setXML(
      "xl/_rels/workbook.xml.rels",
      patchXML(
        relationships.source,
        relations.map((node) => ({
          start: node.start,
          end: node.end,
          value: "",
        })),
      ),
    );
  if (deleted.size) {
    const types = archive.xml(CONTENT_TYPES_PART);
    archive.setXML(
      CONTENT_TYPES_PART,
      patchXML(
        types.source,
        types.root.children
          .filter((node) => deleted.has(node.attributes.PartName ?? ""))
          .map((node) => ({ start: node.start, end: node.end, value: "" })),
      ),
    );
  }
}

interface CellEdit {
  reference: string;
  position: CellAddress;
  expect: string;
  value: CellValue;
}

function editsBySheet(
  operations: Array<{ id: string; expect: string; value: CellValue }>,
  sheets: Set<string>,
): Map<string, CellEdit[]> {
  const seen = new Set<string>();
  const bySheet = new Map<string, CellEdit[]>();
  for (const operation of operations) {
    if (seen.has(operation.id))
      throw new Error("one cell may be changed only once per revision");
    seen.add(operation.id);
    const separator = operation.id.lastIndexOf("!");
    const part = operation.id.slice(0, separator);
    if (separator < 0 || !sheets.has(part))
      throw new Error("cell identifier does not belong to this workbook");
    const reference = operation.id.slice(separator + 1);
    const edits = bySheet.get(part) ?? [];
    edits.push({
      reference,
      position: cellAddress(reference),
      expect: operation.expect,
      value: operation.value,
    });
    bySheet.set(part, edits);
  }
  return bySheet;
}

function worksheetRows(sheetData: XmlNode): Map<number, XmlNode> {
  const rows = new Map<number, XmlNode>();
  for (const row of sheetData.children) {
    const number = Number(row.attributes.r);
    if (
      !isSheetElement(row, "row") ||
      !Number.isInteger(number) ||
      number < 1 ||
      number > xlsxBounds.rows ||
      rows.has(number)
    )
      throw new Error("invalid or duplicate worksheet row");
    rows.set(number, row);
  }
  return rows;
}

export type ProtectionReason = "merge" | "table" | "formulaRegion";

// Cells that setCell refuses to change. A merged range stays editable at its anchor.
export interface ProtectedRegion {
  range: CellRange;
  reason: ProtectionReason;
  anchor?: CellAddress;
}

const protectionRefusals: Record<ProtectionReason, string> = {
  merge: "only the anchor cell of a merged range can be edited",
  table:
    "table headers, totals and calculated columns require a table-aware edit and cannot be changed with setCell",
  formulaRegion:
    "editing a shared, array or data-table formula region is not supported",
};

export function protectedRegions(
  archive: OfficePackage,
  part: string,
  document: XmlDocument,
  cells: Map<string, XmlNode>,
): ProtectedRegion[] {
  const regions: ProtectedRegion[] = [];
  for (const merge of document.nodes.filter((node) =>
    isSheetElement(node, "mergeCell"),
  )) {
    const range = cellRange(merge.attributes.ref!);
    regions.push({ range, reason: "merge", anchor: range.start });
  }
  const tableParts = document.nodes.filter((node) =>
    isSheetElement(node, "tablePart"),
  );
  const relations = tableParts.length
    ? archive.xml(relationshipsPath(part)).root.children
    : [];
  for (const tablePart of tableParts) {
    const relation = relations.find(
      (node) =>
        node.attributes.Id ===
        attribute(tablePart, namespaces.officeRelationships, "id"),
    );
    if (!relation?.attributes.Type?.endsWith("/table"))
      throw new Error("worksheet table relationship is invalid");
    const table = archive.xml(
      relationshipTarget(part, relation.attributes.Target!),
    );
    const { start, end } = cellRange(table.root.attributes.ref!);
    const headerCount = Number(table.root.attributes.headerRowCount ?? "1");
    const totalsCount = Number(table.root.attributes.totalsRowCount ?? "0");
    if (headerCount > 0)
      regions.push({
        range: {
          start,
          end: { row: start.row + headerCount - 1, column: end.column },
        },
        reason: "table",
      });
    if (totalsCount > 0)
      regions.push({
        range: {
          start: { row: end.row - totalsCount + 1, column: start.column },
          end,
        },
        reason: "table",
      });
    table.nodes
      .filter((node) => isSheetElement(node, "tableColumn"))
      .forEach((node, offset) => {
        if (
          node.children.some(
            (child) => child.local === "calculatedColumnFormula",
          )
        )
          regions.push({
            range: {
              start: { row: start.row, column: start.column + offset },
              end: { row: end.row, column: start.column + offset },
            },
            reason: "table",
          });
      });
  }
  for (const cell of cells.values()) {
    const formula = cell.children.find((node) => isSheetElement(node, "f"));
    const ref = formula?.attributes.ref;
    if (
      !ref ||
      !["shared", "array", "dataTable"].includes(formula.attributes.t ?? "")
    )
      continue;
    regions.push({
      range: cellRange(ref.includes(":") ? ref : `${ref}:${ref}`),
      reason: "formulaRegion",
    });
  }
  return regions;
}

function protects(region: ProtectedRegion, point: CellAddress): boolean {
  return (
    rangeContains(region.range, point) &&
    !(
      region.anchor &&
      region.anchor.row === point.row &&
      region.anchor.column === point.column
    )
  );
}

function assertUnprotected(
  regions: ProtectedRegion[],
  points: CellAddress[],
): void {
  for (const region of regions)
    if (points.some((point) => protects(region, point)))
      throw new Error(protectionRefusals[region.reason]);
}

function joinByPosition(entries: Map<number, string>): string {
  return [...entries]
    .sort(([a], [b]) => a - b)
    .map(([, value]) => value)
    .join("");
}

function rowMarkup(
  document: XmlDocument,
  sheetData: XmlNode,
  row: XmlNode | undefined,
  number: number,
  changes: Map<number, string>,
): string {
  if (!row) {
    const name = tagName(sheetData, "row");
    return `<${name} r="${number}">${joinByPosition(changes)}</${name}>`;
  }
  const values = new Map<number, string>();
  const extras = [];
  for (const child of row.children) {
    const markup = document.source.slice(child.start, child.end);
    if (isSheetElement(child, "c"))
      values.set(cellAddress(child.attributes.r!).column, markup);
    else extras.push(markup);
  }
  for (const [column, value] of changes) values.set(column, value);
  const attrs = { ...row.attributes };
  delete attrs.spans;
  return `${openTag(row, attrs)}${joinByPosition(values)}${extras.join("")}</${row.name}>`;
}

function editWorksheet(
  archive: OfficePackage,
  part: string,
  edits: CellEdit[],
  strings: string[],
): void {
  const document = archive.xml(part);
  const cells = worksheetCells(document);
  const sheetData = document.root.children.find((node) =>
    isSheetElement(node, "sheetData"),
  );
  if (!sheetData) throw new Error("worksheet has no sheet data");
  const rows = worksheetRows(sheetData);
  const points = edits.map((edit) => edit.position);
  assertUnprotected(protectedRegions(archive, part, document, cells), points);
  const updated = new Map<number, Map<number, string>>();
  for (const { reference, position, expect, value } of edits) {
    const existing = cells.get(reference);
    if (cellDescription(existing, strings) !== expect)
      throw new Error("cell precondition does not match the exact revision");
    if (
      existing?.children.some(
        (node) =>
          isSheetElement(node, "f") &&
          node.attributes.t &&
          node.attributes.t !== "normal",
      )
    )
      throw new Error(
        "editing a shared, array or data-table formula cell is not supported",
      );
    const row = rows.get(position.row) ?? sheetData;
    const columns = updated.get(position.row) ?? new Map<number, string>();
    columns.set(
      position.column,
      cellMarkup(reference, value, row, existing, document.source),
    );
    updated.set(position.row, columns);
  }
  const body = new Map(
    [...rows].map(([number, row]) => [
      number,
      document.source.slice(row.start, row.end),
    ]),
  );
  for (const [number, changes] of updated)
    body.set(
      number,
      rowMarkup(document, sheetData, rows.get(number), number, changes),
    );
  const patches = [
    {
      start: sheetData.start,
      end: sheetData.end,
      value: `${openTag(sheetData)}${joinByPosition(body)}</${sheetData.name}>`,
    },
  ];
  const dimension = document.root.children.find((node) =>
    isSheetElement(node, "dimension"),
  );
  if (dimension)
    patches.push({ start: dimension.start, end: dimension.end, value: "" });
  archive.setXML(part, patchXML(document.source, patches));
}

export function editWorkbook(
  archive: OfficePackage,
  operations: EditorOperation<"xlsx">[],
): Uint8Array {
  const sheets = new Set(workbookSheets(archive).map((sheet) => sheet.part));
  const strings = sharedStrings(archive);
  for (const [part, edits] of editsBySheet(operations, sheets))
    editWorksheet(archive, part, edits, strings);
  clearCalculation(archive);
  return archive.save();
}
