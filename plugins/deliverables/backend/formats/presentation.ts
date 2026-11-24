import PptxGenJS from "pptxgenjs";
import {
  parseContent,
  presentationContentSchema,
  presentationEditSchema,
  type Inspection,
  type PresentationOperation,
  type SlideElement,
} from "../../contracts/content";
import { canonicalJSON } from "@frame/sdk/json";
import { OfficePackage, relations, relationshipsPath } from "./archive";
import { setChartData } from "./charts";
import { imageBytes } from "./images";
import { inspectOfficeText, officeReport } from "./inspection";
import {
  pruneParts,
  reachableParts,
  restructureSlides,
} from "./slide-structure";
import {
  chartPart,
  child,
  elementFrame,
  propertiesNode,
  resolveElement,
  slideList,
  transformNode,
} from "./slides";
import { assertPermutation, textPatches } from "./text-edits";
import {
  namespaces,
  openTag,
  patchXML,
  type XmlDocument,
  type XmlNode,
  type XmlPatch,
} from "./xml";

const CHART_COLORS = ["245B78", "39A597", "DF9D3A", "8B6FB3"];

function position(element: SlideElement) {
  return { x: element.x, y: element.y, w: element.width, h: element.height };
}
function textOptions(
  element: Extract<SlideElement, { type: "text" }>,
  font: string,
  language?: string,
): PptxGenJS.TextPropsOptions {
  return {
    ...position(element),
    fontFace: font,
    lang: language ?? "en-US",
    fontSize: element.fontSize ?? 22,
    bold: element.bold,
    italic: element.italic,
    color: element.color ?? "183B4E",
    align: element.align,
    bullet: element.bullet ? {} : undefined,
    hyperlink: element.url ? { url: element.url } : undefined,
    breakLine: false,
    margin: 0,
    valign: "top",
  };
}
function shapeOptions(
  element: Extract<SlideElement, { type: "shape" }>,
): PptxGenJS.ShapeProps {
  return {
    ...position(element),
    fill: element.fill
      ? { color: element.fill }
      : { color: "FFFFFF", transparency: 100 },
    line: { color: element.line ?? element.fill ?? "245B78", width: 1 },
  };
}
function imageOptions(
  element: Extract<SlideElement, { type: "image" }>,
): PptxGenJS.ImageProps {
  imageBytes(element.image);
  return {
    ...position(element),
    data: `${element.image.mime};base64,${element.image.base64}`,
    altText: element.image.alt,
  };
}

export async function createPresentation(json: string): Promise<Uint8Array> {
  const content = parseContent(presentationContentSchema, json);
  const presentation = new PptxGenJS();
  presentation.layout =
    content.layout === "standard" ? "LAYOUT_4X3" : "LAYOUT_WIDE";
  presentation.author = content.author ?? "Flame";
  presentation.title = content.title;
  presentation.subject = content.subject ?? "";
  const font = content.font ?? "Aptos";
  presentation.theme = { headFontFace: font, bodyFontFace: font };
  const width = content.layout === "standard" ? 10 : 13.333333;
  const height = 7.5;
  const checkPosition = (element: SlideElement) => {
    if (
      element.x + element.width > width + 0.001 ||
      element.y + element.height > height + 0.001
    )
      throw new Error("slide element exceeds the selected slide dimensions");
  };
  const masters = new Set<string>();
  for (const master of content.masters ?? []) {
    if (masters.has(master.name))
      throw new Error("slide master names must be unique");
    masters.add(master.name);
    const objects: NonNullable<PptxGenJS.SlideMasterProps["objects"]> =
      master.elements.map((element) => {
        checkPosition(element);
        if (element.type === "text")
          return {
            text: {
              text: element.text,
              options: textOptions(element, font, content.language),
            },
          };
        if (element.type === "image") return { image: imageOptions(element) };
        return element.shape === "line"
          ? { line: shapeOptions(element) }
          : { rect: shapeOptions(element) };
      });
    presentation.defineSlideMaster({
      title: master.name,
      background: master.background ? { color: master.background } : undefined,
      objects,
    });
  }
  for (const authored of content.slides) {
    if (authored.master && !masters.has(authored.master))
      throw new Error("slide references an undeclared master");
    const slide = presentation.addSlide(
      authored.master ? { masterName: authored.master } : undefined,
    );
    if (authored.background) slide.background = { color: authored.background };
    for (const element of authored.elements) {
      checkPosition(element);
      switch (element.type) {
        case "text":
          slide.addText(
            element.text,
            textOptions(element, font, content.language),
          );
          break;
        case "shape":
          slide.addShape(
            presentation.ShapeType[element.shape],
            shapeOptions(element),
          );
          break;
        case "image":
          slide.addImage(imageOptions(element));
          break;
        case "table": {
          const columns = element.rows[0]!.length;
          if (element.rows.some((row) => row.length !== columns))
            throw new Error("slide table rows must have equal column counts");
          slide.addTable(
            element.rows.map((row, index) =>
              row.map((text) => ({
                text,
                options:
                  index === 0 && element.header
                    ? { bold: true, color: "FFFFFF", fill: { color: "245B78" } }
                    : {},
              })),
            ),
            {
              ...position(element),
              fontFace: font,
              fontSize: element.fontSize ?? 14,
              border: { type: "solid", color: "D3DEE5", pt: 0.5 },
              margin: 0.08,
              autoPage: false,
              rowH: element.height / element.rows.length,
            },
          );
          break;
        }
        case "chart": {
          if (
            element.series.some(
              (series) => series.labels.length !== series.values.length,
            )
          )
            throw new Error(
              "slide chart labels and values must have equal lengths",
            );
          if (
            ["pie", "doughnut"].includes(element.chart) &&
            element.series.length !== 1
          )
            throw new Error(
              "pie and doughnut charts require exactly one series",
            );
          const values = element.series.flatMap((series) => series.values);
          slide.addChart(
            presentation.ChartType[element.chart],
            element.series,
            {
              ...position(element),
              lang: content.language,
              showTitle: !!element.title,
              title: element.title,
              showLegend: true,
              legendPos: "b",
              showValue: false,
              chartColors: CHART_COLORS,
              catAxisLabelFontFace: font,
              valAxisLabelFontFace: font,
              valAxisMinVal:
                element.chart === "bar" && values.every((value) => value >= 0)
                  ? 0
                  : undefined,
              valAxisMaxVal:
                element.chart === "bar" && values.every((value) => value <= 0)
                  ? 0
                  : undefined,
            },
          );
          break;
        }
        case "scatter": {
          if (
            element.series.some(
              (series) => series.values.length !== element.xValues.length,
            )
          )
            throw new Error(
              "every scatter series needs one y value for each shared x value",
            );
          slide.addChart(
            presentation.ChartType.scatter,
            [{ name: "x", values: element.xValues }, ...element.series],
            {
              ...position(element),
              lang: content.language,
              showTitle: !!element.title,
              title: element.title,
              showLegend: true,
              legendPos: "b",
              chartColors: CHART_COLORS,
              catAxisLabelFontFace: font,
              valAxisLabelFontFace: font,
            },
          );
          break;
        }
      }
    }
    if (authored.notes) slide.addNotes(authored.notes);
  }
  const result = await presentation.write({
    outputType: "uint8array",
    compression: true,
  });
  if (!(result instanceof Uint8Array))
    throw new Error("presentation encoder returned an unexpected result");
  const archive = OfficePackage.open("pptx", result);
  content.slides.forEach((slide, index) => {
    if (!slide.title) return;
    const path = `ppt/slides/slide${index + 1}.xml`;
    const document = archive.xml(path);
    const common = document.root.children.find(
      (node) => node.uri === namespaces.presentation && node.local === "cSld",
    );
    if (!common)
      throw new Error("presentation encoder omitted the slide content");
    archive.setXML(
      path,
      patchXML(document.source, [
        {
          start: common.start,
          end: common.openEnd,
          value: openTag(common, { ...common.attributes, name: slide.title }),
        },
      ]),
    );
  });
  return archive.save();
}

export function inspectPresentation(archive: OfficePackage): Inspection {
  const report = officeReport(archive);
  const { slides } = slideList(archive);
  for (const [index, slide] of slides.entries()) {
    const document = archive.xml(slide.id);
    const common = document.root.children.find(
      (node) => node.uri === namespaces.presentation && node.local === "cSld",
    );
    report.item(
      slide.id,
      "slide",
      common?.attributes.name ?? `Slide ${index + 1}`,
    );
  }
  inspectOfficeText(archive, report);
  Object.assign(report.result.facts, {
    slides: slides.length,
    charts: [...archive.parts.keys()].filter((part) =>
      /^ppt\/charts\/chart[0-9]+\.xml$/u.test(part),
    ).length,
    masters: [...archive.parts.keys()].filter((part) =>
      /^ppt\/slideMasters\/slideMaster[0-9]+\.xml$/u.test(part),
    ).length,
  });
  return report.finish();
}

type Operation<T extends PresentationOperation["type"]> = Extract<
  PresentationOperation,
  { type: T }
>;

function drawingName(document: XmlDocument, local: string) {
  const prefix = Object.entries(document.root.attributes).find(
    ([name, value]) =>
      name.startsWith("xmlns:") && value === namespaces.drawing,
  )?.[0];
  return prefix
    ? { name: `${prefix.slice(6)}:${local}`, declaration: "" }
    : { name: `a:${local}`, declaration: ` xmlns:a="${namespaces.drawing}"` };
}

function framePatch(
  archive: OfficePackage,
  part: string,
  document: XmlDocument,
  node: XmlNode,
  operation: Operation<"setFrame">,
): XmlPatch {
  const current = elementFrame(archive, part, node)?.frame;
  if (!current || canonicalJSON(current) !== canonicalJSON(operation.expect))
    throw new Error(
      "element frame precondition does not match the exact revision",
    );
  const { x, y, width, height } = operation.frame;
  const transform = transformNode(node);
  const offset = child(transform, namespaces.drawing, "off");
  const extent = child(transform, namespaces.drawing, "ext");
  if (offset && extent && offset.end <= extent.start)
    return {
      start: offset.start,
      end: extent.end,
      value: `<${offset.name} x="${x}" y="${y}"/>${document.source.slice(offset.end, extent.start)}<${extent.name} cx="${width}" cy="${height}"/>`,
    };
  // An inherited placeholder frame becomes explicit on the slide.
  const properties = propertiesNode(node);
  if (transform || !properties)
    throw new Error("element frame cannot be stated on this element");
  const xfrm = drawingName(document, "xfrm");
  const name = (local: string) => xfrm.name.replace(/xfrm$/u, local);
  const markup = `<${xfrm.name}${xfrm.declaration}><${name("off")} x="${x}" y="${y}"/><${name("ext")} cx="${width}" cy="${height}"/></${xfrm.name}>`;
  return properties.selfClosing
    ? {
        start: properties.start,
        end: properties.end,
        value: `${openTag(properties)}${markup}</${properties.name}>`,
      }
    : { start: properties.openEnd, end: properties.openEnd, value: markup };
}

function referencedRelationships(node: XmlNode, found = new Set<string>()) {
  for (const [name, value] of Object.entries(node.qualifiedAttributes))
    if (name.startsWith(`${namespaces.officeRelationships}|`)) found.add(value);
  for (const entry of node.children) referencedRelationships(entry, found);
  return found;
}

// Every identifier names the exact source revision; one revision applies the
// whole set or none of it.
export async function editPresentation(
  archive: OfficePackage,
  json: string,
): Promise<Uint8Array> {
  const { operations } = parseContent(presentationEditSchema, json);
  const of = <T extends PresentationOperation["type"]>(...types: T[]) =>
    operations.filter((operation): operation is Operation<T> =>
      (types as string[]).includes(operation.type),
    );
  const structural = of("duplicateSlide", "deleteSlide");
  if (structural.length) {
    if (structural.length !== operations.length)
      throw new Error(
        "duplicateSlide and deleteSlide need a revision without other operation types",
      );
    restructureSlides(archive, structural);
    return archive.save();
  }
  const targets = of("setFrame", "deleteElement", "setChartData").map(
    (operation) => `${operation.type} ${operation.id}`,
  );
  if (new Set(targets).size !== targets.length)
    throw new Error(
      "one element property may be changed only once per revision",
    );
  const patches = textPatches(archive, of("replaceText"));
  const add = (part: string, patch: XmlPatch) =>
    patches.set(part, [...(patches.get(part) ?? []), patch]);
  const deleted = new Set<string>();
  const released = new Map<string, Set<string>>();
  for (const operation of of("deleteElement")) {
    const { part, element } = resolveElement(archive, operation.id);
    if (
      (patches.get(part) ?? []).some(
        (patch) =>
          patch.start >= element.node.start && patch.end <= element.node.end,
      )
    )
      throw new Error("a deleted element cannot also be edited");
    deleted.add(operation.id);
    add(part, { start: element.node.start, end: element.node.end, value: "" });
    released.set(
      part,
      referencedRelationships(element.node, released.get(part)),
    );
  }
  for (const operation of of("setFrame")) {
    if (deleted.has(operation.id))
      throw new Error("a deleted element cannot also be edited");
    const { part, document, element } = resolveElement(archive, operation.id);
    add(part, framePatch(archive, part, document, element.node, operation));
  }
  const charts = of("setChartData").map((operation) => {
    if (deleted.has(operation.id))
      throw new Error("a deleted element cannot also be edited");
    const { part, element } = resolveElement(archive, operation.id);
    const chart = chartPart(archive, part, element.node);
    if (!chart) throw new Error("setChartData requires a chart element");
    return { chart, operation };
  });
  const reachable = deleted.size ? reachableParts(archive) : undefined;
  for (const [part, edits] of patches) archive.patch(part, edits);
  for (const { chart, operation } of charts)
    await setChartData(archive, chart, operation.expect, operation.data);
  for (const [part, candidates] of released) {
    const remaining = referencedRelationships(archive.xml(part).root);
    archive.patch(
      relationshipsPath(part),
      relations(archive, part)
        .filter(
          (relation) =>
            candidates.has(relation.id) && !remaining.has(relation.id),
        )
        .map((relation) => ({
          start: relation.node.start,
          end: relation.node.end,
          value: "",
        })),
    );
  }
  if (reachable) pruneParts(archive, reachable);
  const reorders = of("reorderSlides");
  if (reorders.length > 1)
    throw new Error("slides may be reordered only once per revision");
  if (reorders[0]) {
    const { document, list, slides } = slideList(archive);
    assertPermutation(
      slides.map((slide) => slide.id),
      reorders[0].ids,
    );
    const values = new Map(
      slides.map((slide) => [
        slide.id,
        document.source.slice(slide.node.start, slide.node.end),
      ]),
    );
    archive.patch("ppt/presentation.xml", [
      {
        start: list.openEnd,
        end: list.closeStart,
        value: reorders[0].ids.map((id) => values.get(id)!).join(""),
      },
    ]);
  }
  return archive.save();
}
