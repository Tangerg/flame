import ExcelJS from "exceljs";
import {
  contentLimits,
  parseContent,
  workbookContentSchema,
  type CellValue,
  type WorkbookContent,
} from "../../contracts/content";
import { assertSafeFormula, OfficePackage } from "./archive";
import { HOUSE_STYLE } from "./style";
import { addWorkbookCharts } from "./workbook-charts";
import {
  addressName,
  cellAddress,
  cellRange,
  rangeAddresses,
} from "./workbook-cells";

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
