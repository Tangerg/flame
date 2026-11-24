import ExcelJS from "exceljs";
import {
  contentLimits,
  parseContent,
  workbookContentSchema,
  workbookEditSchema,
  type CellValue,
  type Inspection,
  type WorkbookContent,
} from "../../contracts/content";
import {
  assertSafeFormula,
  OfficePackage,
  relationshipTarget,
  relationshipsPath,
} from "./archive";
import { officeReport } from "./inspection";
import { HOUSE_STYLE } from "./style";
import { addWorkbookCharts } from "./workbook-charts";
import {
  addressName,
  cellAddress,
  cellDescription,
  cellRange,
  cellValue,
  isSheetElement,
  rangeAddresses,
  rangeContains,
  sharedStrings,
  workbookSheets,
  xlsxBounds,
  type CellAddress,
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

function excelValue(value: CellValue): ExcelJS.CellValue {
  switch (value.type) {
    case "formula":
      assertSafeFormula(value.value);
      return { formula: value.value.slice(1) };
    case "blank":
      return null;
    default:
      return value.value;
  }
}

type AuthoredSheet = WorkbookContent["sheets"][number];
type AuthoredStyle = NonNullable<AuthoredSheet["cells"][number]["style"]>;

function addWorksheet(
  book: ExcelJS.Workbook,
  sheet: AuthoredSheet,
): ExcelJS.Worksheet {
  return book.addWorksheet(sheet.name, {
    views: [
      {
        state: "frozen",
        xSplit: sheet.freezeColumns ?? 0,
        ySplit: sheet.freezeRows ?? 0,
      },
    ],
    pageSetup: {
      orientation: sheet.landscape ? "landscape" : "portrait",
      paperSize: 9,
      horizontalDpi: 300,
      verticalDpi: 300,
      fitToPage: true,
      fitToWidth: 1,
      fitToHeight: 0,
    },
  });
}

function styleCell(cell: ExcelJS.Cell, style: AuthoredStyle): void {
  cell.font = {
    name: HOUSE_STYLE.font,
    size: style.fontSize ?? 11,
    bold: style.bold,
    italic: style.italic,
    color: style.color ? { argb: `FF${style.color}` } : undefined,
  };
  if (style.fill)
    cell.fill = {
      type: "pattern",
      pattern: "solid",
      fgColor: { argb: `FF${style.fill}` },
    };
  if (style.numberFormat) cell.numFmt = style.numberFormat;
  cell.alignment = {
    horizontal: style.horizontal,
    wrapText: style.wrap,
    vertical: "middle",
  };
  if (style.border) {
    const border = {
      style: "thin" as const,
      color: { argb: `FF${HOUSE_STYLE.rule}` },
    };
    cell.border = { top: border, bottom: border, left: border, right: border };
  }
}

function writeCells(worksheet: ExcelJS.Worksheet, sheet: AuthoredSheet): void {
  const addresses = new Set<string>();
  for (const authored of sheet.cells) {
    const position = cellAddress(authored.address);
    if (
      position.column > contentLimits.authoredColumns ||
      position.row > contentLimits.authoredRows
    )
      throw new Error(
        `authored cells are limited to ${contentLimits.authoredColumns} columns and ${contentLimits.authoredRows} rows`,
      );
    if (addresses.has(authored.address))
      throw new Error("one authored cell has more than one value");
    addresses.add(authored.address);
    const cell = worksheet.getCell(authored.address);
    cell.value = excelValue(authored.value);
    if (authored.note) cell.note = authored.note;
    if (authored.style) styleCell(cell, authored.style);
  }
}

// Returns the addresses the sheet's tables occupy.
function addTables(
  worksheet: ExcelJS.Worksheet,
  sheet: AuthoredSheet,
  tableNames: Set<string>,
): Set<string> {
  const occupied = new Set<string>();
  for (const table of sheet.tables ?? []) {
    const key = table.name.toLocaleLowerCase("en-US");
    if (tableNames.has(key) || /^[A-Z]{1,3}[0-9]+$/iu.test(table.name))
      throw new Error(
        "table names must be unique and cannot be cell references",
      );
    tableNames.add(key);
    const addresses = rangeAddresses(table.range);
    if (addresses.some((address) => occupied.has(address)))
      throw new Error("workbook tables must not overlap");
    addresses.forEach((address) => occupied.add(address));
    const { start, end } = cellRange(table.range);
    if (start.row === end.row)
      throw new Error(
        "workbook tables require a header and at least one data row",
      );
    const headers: string[] = [];
    for (let column = start.column; column <= end.column; column++) {
      const value = worksheet.getCell(start.row, column).value;
      if (typeof value !== "string" || !value || headers.includes(value))
        throw new Error("table headers must be nonempty distinct string cells");
      headers.push(value);
    }
    const rows = [];
    for (let row = start.row + 1; row <= end.row; row++) {
      const values = [];
      for (let column = start.column; column <= end.column; column++)
        values.push(worksheet.getCell(row, column).value);
      rows.push(values);
    }
    worksheet.addTable({
      name: table.name,
      ref: addressName(start.column, start.row),
      headerRow: true,
      totalsRow: false,
      style: { theme: "TableStyleMedium2", showRowStripes: true },
      columns: headers.map((name) => ({ name, filterButton: true })),
      rows,
    });
  }
  return occupied;
}

function mergeCells(
  worksheet: ExcelJS.Worksheet,
  sheet: AuthoredSheet,
  occupied: Set<string>,
): void {
  for (const merge of sheet.merges ?? []) {
    const addresses = rangeAddresses(merge);
    if (addresses.some((address) => occupied.has(address)))
      throw new Error("merged cells cannot intersect a table");
    if (
      addresses
        .slice(1)
        .some((address) => worksheet.getCell(address).value !== null)
    )
      throw new Error("merging cells would discard authored values");
    worksheet.mergeCells(merge);
  }
}

export async function createWorkbook(json: string): Promise<Uint8Array> {
  const content = parseContent(workbookContentSchema, json);
  if (
    content.sheets.reduce((sum, sheet) => sum + sheet.cells.length, 0) >
    contentLimits.cells
  )
    throw new Error("workbook exceeds the authored cell limit");
  const names = new Set<string>();
  const tableNames = new Set<string>();
  const book = new ExcelJS.Workbook();
  book.creator = content.author ?? HOUSE_STYLE.author;
  book.title = content.title;
  book.calcProperties.fullCalcOnLoad = true;
  for (const sheet of content.sheets) {
    const key = sheet.name.toLocaleLowerCase("en-US");
    if (
      names.has(key) ||
      sheet.name.startsWith("'") ||
      sheet.name.endsWith("'")
    )
      throw new Error(
        "worksheet names must be unique and cannot start or end with an apostrophe",
      );
    names.add(key);
    const worksheet = addWorksheet(book, sheet);
    writeCells(worksheet, sheet);
    for (const column of sheet.columns ?? [])
      worksheet.getColumn(column.column).width = column.width;
    if (sheet.autoFilter) {
      cellRange(sheet.autoFilter);
      worksheet.autoFilter = sheet.autoFilter;
    }
    mergeCells(worksheet, sheet, addTables(worksheet, sheet, tableNames));
  }
  const bytes = new Uint8Array(await book.xlsx.writeBuffer());
  const archive = OfficePackage.open("xlsx", bytes);
  addWorkbookCharts(archive, content);
  const result = archive.save();
  OfficePackage.open("xlsx", result);
  return result;
}

export function worksheetCells(document: XmlDocument): Map<string, XmlNode> {
  const result = new Map<string, XmlNode>();
  for (const node of document.nodes.filter((node) =>
    isSheetElement(node, "c"),
  )) {
    const address = node.attributes.r;
    if (!address || result.has(address))
      throw new Error("worksheet contains missing or duplicate cell addresses");
    cellAddress(address);
    result.set(address, node);
  }
  return result;
}

export function inspectWorkbook(archive: OfficePackage): Inspection {
  const report = officeReport(archive);
  const strings = sharedStrings(archive);
  const sheets = workbookSheets(archive);
  let cells = 0;
  let formulas = 0;
  let missing = 0;
  let errors = 0;
  let decimalText = 0;
  for (const sheet of sheets) {
    report.item(sheet.part, "sheet", sheet.name);
    const document = archive.xml(sheet.part);
    for (const [address, cell] of worksheetCells(document)) {
      const id = `${sheet.part}!${address}`;
      const description = cellDescription(cell, strings);
      report.item(id, "cell", description);
      if (JSON.parse(description).representation === "native_decimal")
        decimalText++;
      cells++;
      const formula = cell.children.find((node) => isSheetElement(node, "f"));
      if (formula) {
        formulas++;
        if (
          !cell.children.some((node) => isSheetElement(node, "v")) ||
          cellValue(cell, strings).type === "blank"
        )
          missing++;
      }
      if (cell.attributes.t === "e") {
        errors++;
        report.check(
          "formula_error",
          "error",
          `Spreadsheet error: ${cell.children.find((node) => node.local === "v")?.text ?? "unknown"}`,
          id,
        );
      }
    }
  }
  Object.assign(report.result.facts, {
    sheets: sheets.length,
    cells,
    formulas,
    missingFormulaCaches: missing,
    formulaErrors: errors,
    charts: [...archive.parts.keys()].filter((part) =>
      /^xl\/charts\/chart[0-9]+\.xml$/u.test(part),
    ).length,
  });
  if (missing)
    report.check(
      "formula_cache_missing",
      "warning",
      `${missing} formula cells have no calculated result. Recalculate the exact revision before delivery.`,
    );
  else if (formulas)
    report.check(
      "formula_cache_observed",
      "info",
      "Formula caches are present. Imported caches are observations, not proof of fresh calculation.",
    );
  if (decimalText)
    report.check(
      "native_decimal_text",
      "warning",
      `${decimalText} numeric cells are projected as their exact native decimal text because JavaScript numeric projection would lose information.`,
    );
  return report.finish();
}

function cellMarkup(
  reference: string,
  value: CellValue,
  row: XmlNode,
  existing: XmlNode | undefined,
  source: string,
): string {
  const attributes: Record<string, string> = {
    ...existing?.attributes,
    r: reference,
  };
  delete attributes.t;
  const name = existing?.name ?? tagName(row, "c");
  const childName = (local: string) =>
    existing ? tagName(existing, local) : tagName(row, local);
  let body = "";
  if (value.type === "string") {
    attributes.t = "inlineStr";
    body = `<${childName("is")}><${childName("t")} xml:space="preserve">${escapeXML(value.value)}</${childName("t")}></${childName("is")}>`;
  } else if (value.type === "formula") {
    assertSafeFormula(value.value);
    body = `<${childName("f")}>${escapeXML(value.value.slice(1))}</${childName("f")}>`;
  } else if (value.type === "number" || value.type === "boolean") {
    if (value.type === "boolean") attributes.t = "b";
    body = `<${childName("v")}>${value.type === "boolean" ? (value.value ? "1" : "0") : value.value}</${childName("v")}>`;
  }
  const retained =
    existing?.children
      .filter(
        (node) =>
          !(
            node.uri === namespaces.sheet &&
            ["f", "v", "is"].includes(node.local)
          ),
      )
      .map((node) => source.slice(node.start, node.end))
      .join("") ?? "";
  return `<${name}${Object.entries(attributes)
    .map(([key, value]) => ` ${key}="${escapeXML(value)}"`)
    .join("")}>${body}${retained}</${name}>`;
}

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
  const workbook = archive.xml("xl/workbook.xml");
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
    "xl/workbook.xml",
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
      "xl/workbook.xml",
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
    const types = archive.xml("[Content_Types].xml");
    archive.setXML(
      "[Content_Types].xml",
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

function assertOutsideMergedCells(
  document: XmlDocument,
  points: CellAddress[],
): void {
  for (const merge of document.nodes.filter((node) =>
    isSheetElement(node, "mergeCell"),
  )) {
    const range = cellRange(merge.attributes.ref!);
    if (
      points.some(
        (point) =>
          rangeContains(range, point) &&
          (point.row !== range.start.row ||
            point.column !== range.start.column),
      )
    )
      throw new Error("only the anchor cell of a merged range can be edited");
  }
}

function assertOutsideTableStructure(
  archive: OfficePackage,
  part: string,
  document: XmlDocument,
  points: CellAddress[],
): void {
  const tableParts = document.nodes.filter((node) =>
    isSheetElement(node, "tablePart"),
  );
  if (!tableParts.length) return;
  const relations = archive.xml(relationshipsPath(part)).root.children;
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
    const range = cellRange(table.root.attributes.ref!);
    const headerCount = Number(table.root.attributes.headerRowCount ?? "1");
    const totalsCount = Number(table.root.attributes.totalsRowCount ?? "0");
    const calculated = table.nodes
      .filter((node) => isSheetElement(node, "tableColumn"))
      .map((node) =>
        node.children.some(
          (child) => child.local === "calculatedColumnFormula",
        ),
      );
    if (
      points.some(
        (point) =>
          rangeContains(range, point) &&
          (point.row < range.start.row + headerCount ||
            point.row > range.end.row - totalsCount ||
            calculated[point.column - range.start.column]),
      )
    )
      throw new Error(
        "table headers, totals and calculated columns require a table-aware edit and cannot be changed with setCell",
      );
  }
}

function assertOutsideFormulaRegions(
  cells: Map<string, XmlNode>,
  points: CellAddress[],
): void {
  for (const cell of cells.values()) {
    const formula = cell.children.find((node) => isSheetElement(node, "f"));
    const ref = formula?.attributes.ref;
    if (
      !ref ||
      !["shared", "array", "dataTable"].includes(formula.attributes.t ?? "")
    )
      continue;
    const range = cellRange(ref.includes(":") ? ref : `${ref}:${ref}`);
    if (points.some((point) => rangeContains(range, point)))
      throw new Error(
        "editing a shared, array or data-table formula region is not supported",
      );
  }
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
  assertOutsideMergedCells(document, points);
  assertOutsideTableStructure(archive, part, document, points);
  assertOutsideFormulaRegions(cells, points);
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

export function editWorkbook(archive: OfficePackage, json: string): Uint8Array {
  const { operations } = parseContent(workbookEditSchema, json);
  const sheets = new Set(workbookSheets(archive).map((sheet) => sheet.part));
  const strings = sharedStrings(archive);
  for (const [part, edits] of editsBySheet(operations, sheets))
    editWorksheet(archive, part, edits, strings);
  clearCalculation(archive);
  const result = archive.save();
  OfficePackage.open("xlsx", result);
  return result;
}

export function workbookCSV(archive: OfficePackage, sheetPart: string): string {
  if (!workbookSheets(archive).some((sheet) => sheet.part === sheetPart))
    throw new Error("worksheet does not belong to this workbook");
  const strings = sharedStrings(archive);
  const cells = worksheetCells(archive.xml(sheetPart));
  let lastRow = 0;
  let lastColumn = 0;
  for (const address of cells.keys()) {
    const position = cellAddress(address);
    lastRow = Math.max(lastRow, position.row);
    lastColumn = Math.max(lastColumn, position.column);
  }
  if (lastRow * lastColumn > contentLimits.cells)
    throw new Error("CSV rectangle exceeds the cell limit");
  const rows = [];
  for (let row = 1; row <= lastRow; row++) {
    const values = [];
    for (let column = 1; column <= lastColumn; column++) {
      const cell = cells.get(addressName(column, row));
      let value = "";
      let stringValue = false;
      if (cell) {
        const cached = cell.children.find((node) => isSheetElement(node, "v"));
        const formula = cell.children.some((node) => isSheetElement(node, "f"));
        const decoded = cellValue(cell, strings);
        if (decoded.type === "error")
          throw new Error("CSV export refuses spreadsheet error cells");
        if (formula && (!cached || decoded.type === "blank"))
          throw new Error(
            "CSV export requires calculated formula values without errors",
          );
        value =
          decoded.type === "blank"
            ? ""
            : decoded.type === "number"
              ? cached!.text
              : String(decoded.value);
        stringValue = decoded.type === "string";
      }
      if (/^[=+@\-\t\r]/u.test(value) && stringValue) value = `'${value}`;
      values.push(`"${value.replace(/"/gu, '""')}"`);
    }
    rows.push(values.join(","));
  }
  return rows.join("\r\n") + "\r\n";
}
