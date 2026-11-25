import {
  contentLimits,
  type CellValue,
  type Inspection,
} from "../../contracts/content";
import { assertSafeFormula, OfficePackage } from "./archive";
import { officeReport } from "./inspection";
import {
  addressName,
  cellAddress,
  cellDescription,
  cellValue,
  isSheetElement,
  sharedStrings,
  workbookSheets,
} from "./workbook-cells";
import {
  escapeXML,
  namespaces,
  tagName,
  type XmlDocument,
  type XmlNode,
} from "./xml";

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

export function cellMarkup(
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
