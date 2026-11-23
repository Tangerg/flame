import { canonicalJSON, parseJSON, projectNumber } from "@frame/sdk/json";
import {
  chartDataSchema,
  type CHART_TYPES,
  type ChartData,
} from "../../contracts/content";
import { OfficePackage, relations } from "./archive";
import { createWorkbook, worksheetCells } from "./workbook";
import {
  cellValue,
  rangeAddresses,
  sharedStrings,
  workbookSheets,
} from "./workbook-cells";
import {
  attribute,
  escapeXML,
  namespaces,
  openTag,
  tagName,
  type XmlDocument,
  type XmlNode,
  type XmlPatch,
} from "./xml";

const GROUPS: Record<string, (typeof CHART_TYPES)[number]> = {
  barChart: "bar",
  bar3DChart: "bar3D",
  lineChart: "line",
  line3DChart: "line3D",
  pieChart: "pie",
  pie3DChart: "pie3D",
  doughnutChart: "doughnut",
  areaChart: "area",
  area3DChart: "area3D",
  radarChart: "radar",
};
const REFERENCES = {
  strRef: "strCache",
  numRef: "numCache",
  multiLvlStrRef: "multiLvlStrCache",
} as const;

function chartChild(node: XmlNode | undefined, local: string) {
  return node?.children.find(
    (entry) => entry.uri === namespaces.chart && entry.local === local,
  );
}

interface SheetRange {
  sheet: string;
  addresses: string[];
}

function sheetRange(formula: string): SheetRange {
  const match =
    /^(?:'((?:[^']|'')+)'|([^'!:()]+))!\$?([A-Z]{1,3})\$?([1-9][0-9]{0,6})(?::\$?([A-Z]{1,3})\$?([1-9][0-9]{0,6}))?$/u.exec(
      formula.trim(),
    );
  if (!match)
    throw new Error("chart data must reference one contiguous worksheet range");
  const start = `${match[3]}${match[4]}`;
  const end = match[5] ? `${match[5]}${match[6]}` : start;
  const addresses = rangeAddresses(`${start}:${end}`, 1000);
  if (match[3] !== (match[5] ?? match[3]) && match[4] !== (match[6] ?? match[4]))
    throw new Error("chart data ranges must be a single row or column");
  return { sheet: match[1]?.replaceAll("''", "'") ?? match[2]!, addresses };
}

interface Reference {
  kind: keyof typeof REFERENCES;
  // The element that directly owns the c:pt entries.
  points: XmlNode;
  values: string[];
  missing: boolean[];
  range: SheetRange;
}

function reference(container: XmlNode | undefined): Reference {
  const owner = container?.children.find(
    (entry) => entry.uri === namespaces.chart && entry.local in REFERENCES,
  );
  if (!owner)
    throw new Error("chart series data must reference worksheet cells");
  const kind = owner.local as keyof typeof REFERENCES;
  const formula = chartChild(owner, "f")?.text;
  const cache = chartChild(owner, REFERENCES[kind]);
  if (!formula || !cache) throw new Error("chart reference has no cached data");
  let points = cache;
  if (kind === "multiLvlStrRef") {
    const levels = cache.children.filter(
      (entry) => entry.uri === namespaces.chart && entry.local === "lvl",
    );
    if (levels.length !== 1)
      throw new Error("multi-level chart categories are not editable");
    points = levels[0]!;
  }
  const count = chartChild(cache, "ptCount")?.attributes.val ?? "";
  if (!/^(?:0|[1-9][0-9]{0,3})$/u.test(count) || Number(count) > 1000)
    throw new Error("chart cache has an invalid point count");
  const values = Array<string>(Number(count)).fill("");
  const missing = Array<boolean>(Number(count)).fill(true);
  for (const point of points.children.filter(
    (entry) => entry.uri === namespaces.chart && entry.local === "pt",
  )) {
    const index = point.attributes.idx ?? "";
    if (
      !/^(?:0|[1-9][0-9]*)$/u.test(index) ||
      Number(index) >= values.length ||
      !missing[Number(index)]
    )
      throw new Error("chart cache has an invalid point index");
    values[Number(index)] = chartChild(point, "v")?.text ?? "";
    missing[Number(index)] = false;
  }
  const range = sheetRange(formula);
  if (range.addresses.length !== values.length)
    throw new Error("chart cache and its worksheet range differ in size");
  return { kind, points, values, missing, range };
}

function exact(lexeme: string): number {
  try {
    return projectNumber(parseJSON(lexeme.trim()));
  } catch {
    throw new Error("chart value cannot be represented exactly");
  }
}

export interface ChartModel {
  document: XmlDocument;
  type: (typeof CHART_TYPES)[number];
  series: Array<{ name: Reference; categories: Reference; values: Reference }>;
  data: ChartData;
  workbook: { kind: "none" } | { kind: "chart" | "mixed"; part: string };
}

export function readChart(archive: OfficePackage, part: string): ChartModel {
  const document = archive.xml(part);
  const plot = document.nodes.find(
    (node) => node.uri === namespaces.chart && node.local === "plotArea",
  );
  const groups =
    plot?.children.filter(
      (node) => node.uri === namespaces.chart && node.local.endsWith("Chart"),
    ) ?? [];
  const type = groups.length === 1 ? GROUPS[groups[0]!.local] : undefined;
  if (!type)
    throw new Error(
      "only a single bar, line, pie, doughnut, area or radar chart group is editable",
    );
  const series = groups[0]!.children
    .filter((node) => node.uri === namespaces.chart && node.local === "ser")
    .map((node) => ({
      name: reference(chartChild(node, "tx")),
      categories: reference(chartChild(node, "cat")),
      values: reference(chartChild(node, "val")),
    }));
  const first = series[0];
  if (
    !first ||
    series.some(
      (entry) =>
        entry.name.kind !== "strRef" ||
        entry.name.values.length !== 1 ||
        entry.values.kind !== "numRef" ||
        entry.values.values.length !== first.categories.values.length ||
        canonicalJSON(entry.categories.values) !==
          canonicalJSON(first.categories.values) ||
        canonicalJSON(entry.categories.range) !==
          canonicalJSON(first.categories.range),
    )
  )
    throw new Error(
      "chart series must share their categories and reference one name cell",
    );
  const data = chartDataSchema.parse({
    categories: first.categories.values,
    series: series.map((entry) => ({
      name: entry.name.values[0],
      values: entry.values.values.map((value, index) =>
        entry.values.missing[index] || value === "" ? null : exact(value),
      ),
    })),
  });
  return {
    document,
    type,
    series,
    data,
    workbook: embeddedWorkbook(archive, part, document, series),
  };
}

function embeddedWorkbook(
  archive: OfficePackage,
  part: string,
  document: XmlDocument,
  series: ChartModel["series"],
): ChartModel["workbook"] {
  const external = chartChild(document.root, "externalData");
  if (!external) return { kind: "none" };
  const id = attribute(external, namespaces.officeRelationships, "id");
  const relation = relations(archive, part).find((entry) => entry.id === id);
  if (relation?.type !== "package" || !relation.target)
    throw new Error("chart external data is not an embedded workbook");
  const referenced = new Map<string, Set<string>>();
  for (const entry of series)
    for (const { range } of [entry.name, entry.categories, entry.values]) {
      const cells = referenced.get(range.sheet) ?? new Set<string>();
      for (const address of range.addresses) cells.add(address);
      referenced.set(range.sheet, cells);
    }
  try {
    const workbook = OfficePackage.open(
      "xlsx",
      archive.parts.get(relation.target)!,
    );
    const strings = sharedStrings(workbook);
    const sheets = workbookSheets(workbook);
    if (
      [...referenced.keys()].some(
        (name) => !sheets.some((sheet) => sheet.name === name),
      )
    )
      return { kind: "mixed", part: relation.target };
    for (const sheet of sheets) {
      const allowed = referenced.get(sheet.name);
      for (const [address, cell] of worksheetCells(workbook.xml(sheet.part))) {
        if (
          cell.children.some(
            (node) => node.uri === namespaces.sheet && node.local === "f",
          )
        )
          return { kind: "mixed", part: relation.target };
        const value = cellValue(cell, strings);
        if (
          value.type === "blank" ||
          (value.type === "string" && value.value === "")
        )
          continue;
        if (!allowed?.has(address))
          return { kind: "mixed", part: relation.target };
      }
    }
  } catch {
    return { kind: "mixed", part: relation.target };
  }
  return { kind: "chart", part: relation.target };
}

function cachePatch(
  source: string,
  reference: Reference,
  values: Array<string | null>,
): XmlPatch {
  const { points } = reference;
  const pointName = tagName(points, "pt");
  const valueName = tagName(points, "v");
  const entries = points.children.filter(
    (entry) => !(entry.uri === namespaces.chart && entry.local === "pt"),
  );
  const leading = entries.filter((entry) =>
    ["formatCode", "ptCount"].includes(entry.local),
  );
  const trailing = entries.filter((entry) => !leading.includes(entry));
  const slice = (entry: XmlNode) => source.slice(entry.start, entry.end);
  const body = [
    ...leading.map(slice),
    ...values.flatMap((value, index) =>
      value === null
        ? []
        : [
            `<${pointName} idx="${index}"><${valueName}>${escapeXML(value)}</${valueName}></${pointName}>`,
          ],
    ),
    ...trailing.map(slice),
  ].join("");
  return {
    start: points.start,
    end: points.end,
    value: `${openTag(points)}${body}</${points.name}>`,
  };
}

function numberLexeme(value: number): string {
  return Object.is(value, -0) ? "0" : String(value);
}

export async function setChartData(
  archive: OfficePackage,
  part: string,
  expect: ChartData,
  data: ChartData,
): Promise<void> {
  const model = readChart(archive, part);
  if (canonicalJSON(model.data) !== canonicalJSON(expect))
    throw new Error("chart data precondition does not match the exact revision");
  if (
    data.categories.length !== model.data.categories.length ||
    data.series.length !== model.series.length ||
    data.series.some(
      (series) => series.values.length !== data.categories.length,
    )
  )
    throw new Error(
      "chart data edits keep the original category and series counts",
    );
  if (model.workbook.kind === "mixed")
    throw new Error(
      "the embedded chart workbook holds data outside the chart ranges; edit it in Office",
    );
  const numericCategories = model.series[0]!.categories.kind === "numRef";
  if (numericCategories) for (const category of data.categories) exact(category);
  archive.patch(
    part,
    model.series.flatMap((series, index) => [
      cachePatch(model.document.source, series.name, [data.series[index]!.name]),
      cachePatch(model.document.source, series.categories, data.categories),
      cachePatch(
        model.document.source,
        series.values,
        data.series[index]!.values.map((value) =>
          value === null ? null : numberLexeme(value),
        ),
      ),
    ]),
  );
  if (model.workbook.kind !== "chart") return;
  // The workbook held only these ranges, so regenerating it loses no data.
  const sheets = new Map<string, Map<string, unknown>>();
  const write = (sheet: string, address: string, value: unknown) => {
    const cells = sheets.get(sheet) ?? new Map<string, unknown>();
    cells.set(address, value);
    sheets.set(sheet, cells);
  };
  model.series.forEach((series, index) => {
    const authored = data.series[index]!;
    write(series.name.range.sheet, series.name.range.addresses[0]!, {
      type: "string",
      value: authored.name,
    });
    series.categories.range.addresses.forEach((address, point) =>
      write(
        series.categories.range.sheet,
        address,
        numericCategories
          ? { type: "number", value: exact(data.categories[point]!) }
          : { type: "string", value: data.categories[point] },
      ),
    );
    series.values.range.addresses.forEach((address, point) => {
      const value = authored.values[point];
      write(
        series.values.range.sheet,
        address,
        value === null ? { type: "blank" } : { type: "number", value },
      );
    });
  });
  archive.parts.set(
    model.workbook.part,
    await createWorkbook(
      JSON.stringify({
        title: "Chart data",
        sheets: [...sheets].map(([name, cells]) => ({
          name,
          cells: [...cells].map(([address, value]) => ({ address, value })),
        })),
      }),
    ),
  );
}
