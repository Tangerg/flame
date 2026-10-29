import { attribute, namespaces, nodeText, type XmlNode } from "./xml";
import { OfficePackage, relationshipTarget } from "./archive";
import type { CellValue } from "../../contracts/content";
import { parseJSON, projectNumber } from "@qimu/sdk/json";

export interface CellAddress {
  column: number;
  row: number;
}
export function cellAddress(value: string): CellAddress {
  const match = /^([A-Z]{1,3})([1-9][0-9]{0,6})$/u.exec(value);
  if (!match) throw new Error("invalid spreadsheet cell address");
  let column = 0;
  for (const letter of match[1]!)
    column = column * 26 + letter.charCodeAt(0) - 64;
  const row = Number(match[2]);
  if (column > 16384 || row > 1048576)
    throw new Error("spreadsheet cell is outside XLSX dimensions");
  return { column, row };
}
export function addressName(column: number, row: number): string {
  let name = "";
  while (column > 0) {
    column--;
    name = String.fromCharCode(65 + (column % 26)) + name;
    column = Math.floor(column / 26);
  }
  return `${name}${row}`;
}
export function cellRange(value: string): {
  start: CellAddress;
  end: CellAddress;
} {
  const [first, last] = value.split(":");
  const start = cellAddress(first!);
  const end = cellAddress(last!);
  if (start.column > end.column || start.row > end.row)
    throw new Error("spreadsheet range bounds are reversed");
  return { start, end };
}
export function rangeAddresses(value: string, maximum = 50_000): string[] {
  const { start, end } = cellRange(value);
  if ((end.row - start.row + 1) * (end.column - start.column + 1) > maximum)
    throw new Error("spreadsheet range exceeds the supported cell count");
  const result = [];
  for (let row = start.row; row <= end.row; row++)
    for (let column = start.column; column <= end.column; column++)
      result.push(addressName(column, row));
  return result;
}
export function sharedStrings(archive: OfficePackage): string[] {
  if (!archive.parts.has("xl/sharedStrings.xml")) return [];
  return archive
    .xml("xl/sharedStrings.xml")
    .root.children.filter(
      (node) => node.uri === namespaces.sheet && node.local === "si",
    )
    .map(nodeText);
}
export function cellDescription(
  cell: XmlNode | undefined,
  strings: string[],
): string {
  if (!cell) return "";
  const formula = cell.children.find(
    (node) => node.uri === namespaces.sheet && node.local === "f",
  );
  if (formula) {
    if (formula.attributes.t === "shared" && !formula.text)
      return JSON.stringify({ type: "formula", shared: formula.attributes.si });
    return JSON.stringify({ type: "formula", value: `=${formula.text}` });
  }
  return JSON.stringify(cellValue(cell, strings));
}

type CellProjection =
  | Exclude<CellValue, { type: "formula" }>
  | { type: "error"; value: string }
  | { type: "number"; value: string; representation: "native_decimal" };

export function cellValue(cell: XmlNode, strings: string[]): CellProjection {
  const value = cell.children.find(
    (node) => node.uri === namespaces.sheet && node.local === "v",
  )?.text;
  let projected: CellProjection;
  switch (cell.attributes.t) {
    case "inlineStr": {
      const inline = cell.children.find(
        (node) => node.local === "is" && node.uri === namespaces.sheet,
      );
      if (!inline) throw new Error("inline string cell has no string content");
      projected = { type: "string", value: nodeText(inline) };
      break;
    }
    case "s": {
      if (
        !value ||
        !/^(?:0|[1-9][0-9]*)$/u.test(value) ||
        strings[Number(value)] === undefined
      )
        throw new Error("spreadsheet references a missing shared string");
      projected = { type: "string", value: strings[Number(value)]! };
      break;
    }
    case "b":
      if (value !== "0" && value !== "1")
        throw new Error("invalid spreadsheet boolean value");
      projected = { type: "boolean", value: value === "1" };
      break;
    case "e":
      projected = { type: "error", value: value ?? "" };
      break;
    case "str":
    case "d":
      projected = { type: "string", value: value ?? "" };
      break;
    case undefined:
    case "n":
      if (value === undefined || value === "") projected = { type: "blank" };
      else {
        if (
          !/^[+-]?(?:[0-9]+(?:\.[0-9]*)?|\.[0-9]+)(?:[eE][+-]?[0-9]+)?$/u.test(
            value,
          ) ||
          !Number.isFinite(Number(value))
        )
          throw new Error("invalid numeric spreadsheet cell");
        try {
          projected = {
            type: "number",
            value: projectNumber(parseJSON(value)),
          };
        } catch (error) {
          if (!(
            error instanceof SyntaxError ||
            error instanceof RangeError ||
            error instanceof TypeError
          ))
            throw error;
          projected = {
            type: "number",
            value,
            representation: "native_decimal",
          };
        }
      }
      break;
    default:
      throw new Error("unsupported spreadsheet cell value type");
  }
  return projected;
}

export function workbookSheets(
  archive: OfficePackage,
): Array<{ name: string; part: string }> {
  const workbook = archive.xml("xl/workbook.xml");
  const relationships = archive.xml("xl/_rels/workbook.xml.rels").root.children;
  const result: Array<{ name: string; part: string }> = [];
  for (const sheet of workbook.nodes.filter(
    (node) => node.uri === namespaces.sheet && node.local === "sheet",
  )) {
    const relation = relationships.find(
      (candidate) =>
        candidate.attributes.Id ===
        attribute(sheet, namespaces.officeRelationships, "id"),
    );
    if (
      !relation ||
      !relation.attributes.Type?.endsWith("/worksheet") ||
      !sheet.attributes.name
    )
      throw new Error("workbook contains an unsupported sheet relationship");
    const part = relationshipTarget(
      "xl/workbook.xml",
      relation.attributes.Target!,
    );
    if (
      result.some(
        (entry) =>
          entry.part === part ||
          entry.name.toLocaleLowerCase("en-US") ===
            sheet.attributes.name!.toLocaleLowerCase("en-US"),
      )
    )
      throw new Error("duplicate worksheet identity");
    result.push({ name: sheet.attributes.name, part });
  }
  return result;
}
