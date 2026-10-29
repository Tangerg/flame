import type {
  WorkbookContent,
  WorkbookChart,
  CellValue,
} from "../../contracts/content";
import { OfficePackage } from "./archive";
import { cellAddress, cellRange, rangeAddresses } from "./workbook-cells";
import { escapeXML, namespaces, patchXML } from "./xml";

const chartType =
  "application/vnd.openxmlformats-officedocument.drawingml.chart+xml";
const drawingType = "application/vnd.openxmlformats-officedocument.drawing+xml";
const palette = ["245B78", "39A597", "DF9D3A", "8B6FB3", "D65C68", "739643"];
const declaration = '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>';

function formula(sheet: string, range: string): string {
  return escapeXML(
    `'${sheet.replace(/'/gu, "''")}'!${range.replace(/([A-Z]+)([0-9]+)/gu, "$$$1$$$2")}`,
  );
}

function chartXML(
  chart: WorkbookChart,
  sheet: string,
  cells: Map<string, CellValue>,
  index: number,
): string {
  const categories = rangeAddresses(chart.categories, 1000);
  const dimension = cellRange(chart.categories);
  if (
    dimension.start.column !== dimension.end.column &&
    dimension.start.row !== dimension.end.row
  )
    throw new Error("chart categories must be a single row or column");
  if (chart.type === "pie" && chart.series.length !== 1)
    throw new Error("pie charts require exactly one series");
  const catPoints = categories
    .map((address, point) => {
      const value = cells.get(address);
      return value && value.type !== "formula" && value.type !== "blank"
        ? `<c:pt idx="${point}"><c:v>${escapeXML(String(value.value))}</c:v></c:pt>`
        : "";
    })
    .join("");
  const series = chart.series
    .map((series, position) => {
      const values = rangeAddresses(series.values, 1000);
      const bounds = cellRange(series.values);
      if (
        values.length !== categories.length ||
        (bounds.start.column !== bounds.end.column &&
          bounds.start.row !== bounds.end.row)
      )
        throw new Error(
          "chart value ranges must be one-dimensional and match categories",
        );
      const points = values
        .map((address, point) => {
          const value = cells.get(address);
          if (
            value &&
            value.type !== "formula" &&
            value.type !== "blank" &&
            value.type !== "number"
          )
            throw new Error(
              "numeric chart series references a nonnumeric cell",
            );
          return value?.type === "number"
            ? `<c:pt idx="${point}"><c:v>${value.value}</c:v></c:pt>`
            : "";
        })
        .join("");
      const color = series.color ?? palette[position % palette.length]!;
      return `<c:ser><c:idx val="${position}"/><c:order val="${position}"/><c:tx><c:v>${escapeXML(series.name)}</c:v></c:tx><c:spPr><a:solidFill><a:srgbClr val="${color}"/></a:solidFill><a:ln><a:solidFill><a:srgbClr val="${color}"/></a:solidFill></a:ln></c:spPr>${chart.type === "line" ? '<c:marker><c:symbol val="circle"/><c:size val="5"/></c:marker>' : ""}<c:cat><c:strRef><c:f>${formula(sheet, chart.categories)}</c:f><c:strCache><c:ptCount val="${categories.length}"/>${catPoints}</c:strCache></c:strRef></c:cat><c:val><c:numRef><c:f>${formula(sheet, series.values)}</c:f><c:numCache><c:formatCode>General</c:formatCode><c:ptCount val="${values.length}"/>${points}</c:numCache></c:numRef></c:val></c:ser>`;
    })
    .join("");
  const catAxis = index * 10 + 1;
  const valueAxis = index * 10 + 2;
  const values = chart.series.flatMap((series) =>
    rangeAddresses(series.values, 1000).map((address) => cells.get(address)),
  );
  const allNumeric = values.every((value) => value?.type === "number");
  const baseline =
    chart.type === "bar" && allNumeric
      ? values.every((value) => value!.type === "number" && value.value >= 0)
        ? '<c:min val="0"/>'
        : values.every((value) => value!.type === "number" && value.value <= 0)
          ? '<c:max val="0"/>'
          : ""
      : "";
  const axes =
    chart.type === "pie"
      ? ""
      : `<c:catAx><c:axId val="${catAxis}"/><c:scaling><c:orientation val="minMax"/></c:scaling><c:axPos val="b"/><c:tickLblPos val="nextTo"/><c:crossAx val="${valueAxis}"/><c:crosses val="autoZero"/><c:auto val="1"/><c:lblAlgn val="ctr"/><c:lblOffset val="100"/></c:catAx><c:valAx><c:axId val="${valueAxis}"/><c:scaling><c:orientation val="minMax"/>${baseline}</c:scaling><c:axPos val="l"/><c:majorGridlines/><c:numFmt formatCode="General" sourceLinked="1"/><c:tickLblPos val="nextTo"/><c:crossAx val="${catAxis}"/><c:crosses val="autoZero"/><c:crossBetween val="between"/></c:valAx>`;
  const type = `${chart.type}Chart`;
  const settings =
    chart.type === "bar"
      ? '<c:barDir val="col"/><c:grouping val="clustered"/><c:varyColors val="0"/>'
      : chart.type === "line"
        ? '<c:grouping val="standard"/><c:varyColors val="0"/>'
        : '<c:varyColors val="1"/>';
  const ids =
    chart.type === "pie"
      ? ""
      : `<c:axId val="${catAxis}"/><c:axId val="${valueAxis}"/>`;
  return `${declaration}<c:chartSpace xmlns:c="${namespaces.chart}" xmlns:a="${namespaces.drawing}" xmlns:r="${namespaces.officeRelationships}"><c:date1904 val="0"/><c:lang val="en-US"/><c:chart><c:title><c:tx><c:rich><a:bodyPr/><a:lstStyle/><a:p><a:r><a:rPr lang="en-US"/><a:t>${escapeXML(chart.title)}</a:t></a:r></a:p></c:rich></c:tx><c:overlay val="0"/></c:title><c:autoTitleDeleted val="0"/><c:plotArea><c:layout/><c:${type}>${settings}${series}${ids}</c:${type}>${axes}</c:plotArea><c:legend><c:legendPos val="b"/><c:overlay val="0"/></c:legend><c:plotVisOnly val="1"/><c:dispBlanksAs val="gap"/></c:chart></c:chartSpace>`;
}

export function addWorkbookCharts(
  archive: OfficePackage,
  content: WorkbookContent,
): void {
  const additions: string[] = [];
  let chartIndex = 0;
  content.sheets.forEach((sheet, sheetIndex) => {
    if (!sheet.charts?.length) return;
    const index = sheetIndex + 1;
    const part = `xl/worksheets/sheet${index}.xml`;
    const drawing = `xl/drawings/drawing${index}.xml`;
    if (archive.parts.has(drawing))
      throw new Error("fresh workbook unexpectedly contains a drawing");
    const sheetXML = archive.xml(part);
    const relationPart = `xl/worksheets/_rels/sheet${index}.xml.rels`;
    const relationID = "flameCharts";
    const drawingRelation = `<Relationship Id="${relationID}" Type="${namespaces.officeRelationships}/drawing" Target="../drawings/drawing${index}.xml"/>`;
    if (archive.parts.has(relationPart)) {
      const relationships = archive.xml(relationPart);
      if (
        relationships.root.children.some(
          (node) => node.attributes.Id === relationID,
        )
      )
        throw new Error("duplicate worksheet drawing relationship");
      archive.setXML(
        relationPart,
        patchXML(relationships.source, [
          {
            start: relationships.root.closeStart,
            end: relationships.root.closeStart,
            value: drawingRelation,
          },
        ]),
      );
    } else
      archive.setXML(
        relationPart,
        `${declaration}<Relationships xmlns="${namespaces.relationships}">${drawingRelation}</Relationships>`,
      );
    const drawingNode = `<drawing xmlns:r="${namespaces.officeRelationships}" r:id="${relationID}"/>`;
    const before = sheetXML.root.children.find((node) =>
      [
        "legacyDrawing",
        "legacyDrawingHF",
        "picture",
        "oleObjects",
        "controls",
        "webPublishItems",
        "tableParts",
        "extLst",
      ].includes(node.local),
    );
    const insertion = before?.start ?? sheetXML.root.closeStart;
    archive.setXML(
      part,
      patchXML(sheetXML.source, [
        { start: insertion, end: insertion, value: drawingNode },
      ]),
    );
    const relationships: string[] = [];
    const frames: string[] = [];
    const cells = new Map(
      sheet.cells.map((cell) => [cell.address, cell.value]),
    );
    sheet.charts.forEach((chart, chartInSheet) => {
      chartIndex++;
      const chartPart = `xl/charts/chart${chartIndex}.xml`;
      archive.setXML(chartPart, chartXML(chart, sheet.name, cells, chartIndex));
      additions.push(
        `<Override PartName="/${chartPart}" ContentType="${chartType}"/>`,
      );
      relationships.push(
        `<Relationship Id="rId${chartInSheet + 1}" Type="${namespaces.officeRelationships}/chart" Target="../charts/chart${chartIndex}.xml"/>`,
      );
      const anchor = cellAddress(chart.anchor);
      frames.push(
        `<xdr:oneCellAnchor><xdr:from><xdr:col>${anchor.column - 1}</xdr:col><xdr:colOff>0</xdr:colOff><xdr:row>${anchor.row - 1}</xdr:row><xdr:rowOff>0</xdr:rowOff></xdr:from><xdr:ext cx="${Math.round((chart.width ?? 640) * 9525)}" cy="${Math.round((chart.height ?? 360) * 9525)}"/><xdr:graphicFrame macro=""><xdr:nvGraphicFramePr><xdr:cNvPr id="${chartInSheet + 1}" name="${escapeXML(chart.title)}"/><xdr:cNvGraphicFramePr/></xdr:nvGraphicFramePr><xdr:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/></xdr:xfrm><a:graphic><a:graphicData uri="${namespaces.chart}"><c:chart xmlns:c="${namespaces.chart}" xmlns:r="${namespaces.officeRelationships}" r:id="rId${chartInSheet + 1}"/></a:graphicData></a:graphic></xdr:graphicFrame><xdr:clientData/></xdr:oneCellAnchor>`,
      );
    });
    archive.setXML(
      drawing,
      `${declaration}<xdr:wsDr xmlns:xdr="http://schemas.openxmlformats.org/drawingml/2006/spreadsheetDrawing" xmlns:a="${namespaces.drawing}">${frames.join("")}</xdr:wsDr>`,
    );
    archive.setXML(
      `xl/drawings/_rels/drawing${index}.xml.rels`,
      `${declaration}<Relationships xmlns="${namespaces.relationships}">${relationships.join("")}</Relationships>`,
    );
    additions.push(
      `<Override PartName="/${drawing}" ContentType="${drawingType}"/>`,
    );
  });
  if (additions.length) {
    const types = archive.xml("[Content_Types].xml");
    archive.setXML(
      "[Content_Types].xml",
      patchXML(types.source, [
        {
          start: types.root.closeStart,
          end: types.root.closeStart,
          value: additions.join(""),
        },
      ]),
    );
  }
}
