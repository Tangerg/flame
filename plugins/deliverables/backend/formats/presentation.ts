import PptxGenJS from "pptxgenjs";
import {
  parseContent,
  presentationContentSchema,
  presentationEditSchema,
  type Inspection,
  type PresentationOperation,
  type PresentationContent,
  type SlideElement,
} from "../../contracts/content";
import { canonicalJSON } from "@frame/sdk/json";
import { OfficePackage, relations, relationshipsPath } from "./archive";
import { setChartData } from "./charts";
import { HOUSE_STYLE } from "./style";
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
    line: {
      color: element.line ?? element.fill ?? HOUSE_STYLE.accent,
      width: 1,
    },
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

// Slide dimensions in inches for each PptxGenJS layout the content may select.
const LAYOUTS = {
  wide: { name: "LAYOUT_WIDE", width: 13.333333, height: 7.5 },
  standard: { name: "LAYOUT_4X3", width: 10, height: 7.5 },
} as const;
// Positions are authored in inches; this absorbs rounding at the slide edge.
const EDGE_TOLERANCE = 0.001;

type Element<T extends SlideElement["type"]> = Extract<
  SlideElement,
  { type: T }
>;

interface Authoring {
  presentation: PptxGenJS;
  font: string;
  language: PresentationContent["language"];
  layout: (typeof LAYOUTS)[keyof typeof LAYOUTS];
}

function checkPosition(element: SlideElement, { layout }: Authoring): void {
  if (
    element.x + element.width > layout.width + EDGE_TOLERANCE ||
    element.y + element.height > layout.height + EDGE_TOLERANCE
  )
    throw new Error("slide element exceeds the selected slide dimensions");
}

type MasterElement = NonNullable<
  PresentationContent["masters"]
>[number]["elements"][number];

function masterObject(
  element: MasterElement,
  authoring: Authoring,
): NonNullable<PptxGenJS.SlideMasterProps["objects"]>[number] {
  checkPosition(element, authoring);
  if (element.type === "text")
    return {
      text: {
        text: element.text,
        options: textOptions(element, authoring.font, authoring.language),
      },
    };
  if (element.type === "image") return { image: imageOptions(element) };
  return element.shape === "line"
    ? { line: shapeOptions(element) }
    : { rect: shapeOptions(element) };
}

function defineMasters(
  content: PresentationContent,
  authoring: Authoring,
): Set<string> {
  const masters = new Set<string>();
  for (const master of content.masters ?? []) {
    if (masters.has(master.name))
      throw new Error("slide master names must be unique");
    masters.add(master.name);
    authoring.presentation.defineSlideMaster({
      title: master.name,
      background: master.background ? { color: master.background } : undefined,
      objects: master.elements.map((element) =>
        masterObject(element, authoring),
      ),
    });
  }
  return masters;
}

function addTable(
  slide: PptxGenJS.Slide,
  element: Element<"table">,
  { font }: Authoring,
): void {
  const columns = element.rows[0]!.length;
  if (element.rows.some((row) => row.length !== columns))
    throw new Error("slide table rows must have equal column counts");
  slide.addTable(
    element.rows.map((row, index) =>
      row.map((text) => ({
        text,
        options:
          index === 0 && element.header
            ? {
                bold: true,
                color: HOUSE_STYLE.onAccent,
                fill: { color: HOUSE_STYLE.accent },
              }
            : {},
      })),
    ),
    {
      ...position(element),
      fontFace: font,
      fontSize: element.fontSize ?? 14,
      border: { type: "solid", color: HOUSE_STYLE.rule, pt: 0.5 },
      margin: 0.08,
      autoPage: false,
      rowH: element.height / element.rows.length,
    },
  );
}

function chartFrame(
  element: Element<"chart"> | Element<"scatter">,
  { font, language }: Authoring,
) {
  return {
    ...position(element),
    lang: language,
    showTitle: !!element.title,
    title: element.title,
    showLegend: true,
    legendPos: "b" as const,
    chartColors: [...HOUSE_STYLE.palette],
    catAxisLabelFontFace: font,
    valAxisLabelFontFace: font,
  };
}

function addChart(
  slide: PptxGenJS.Slide,
  element: Element<"chart">,
  authoring: Authoring,
): void {
  if (
    element.series.some(
      (series) => series.labels.length !== series.values.length,
    )
  )
    throw new Error("slide chart labels and values must have equal lengths");
  if (
    ["pie", "doughnut"].includes(element.chart) &&
    element.series.length !== 1
  )
    throw new Error("pie and doughnut charts require exactly one series");
  const values = element.series.flatMap((series) => series.values);
  // A bar axis keeps zero in view when every value sits on one side of it.
  const bar = element.chart === "bar";
  slide.addChart(
    authoring.presentation.ChartType[element.chart],
    element.series,
    {
      ...chartFrame(element, authoring),
      showValue: false,
      valAxisMinVal: bar && values.every((value) => value >= 0) ? 0 : undefined,
      valAxisMaxVal: bar && values.every((value) => value <= 0) ? 0 : undefined,
    },
  );
}

function addScatter(
  slide: PptxGenJS.Slide,
  element: Element<"scatter">,
  authoring: Authoring,
): void {
  if (
    element.series.some(
      (series) => series.values.length !== element.xValues.length,
    )
  )
    throw new Error(
      "every scatter series needs one y value for each shared x value",
    );
  slide.addChart(
    authoring.presentation.ChartType.scatter,
    [{ name: "x", values: element.xValues }, ...element.series],
    chartFrame(element, authoring),
  );
}

function addElement(
  slide: PptxGenJS.Slide,
  element: SlideElement,
  authoring: Authoring,
): void {
  checkPosition(element, authoring);
  switch (element.type) {
    case "text":
      slide.addText(
        element.text,
        textOptions(element, authoring.font, authoring.language),
      );
      return;
    case "shape":
      slide.addShape(
        authoring.presentation.ShapeType[element.shape],
        shapeOptions(element),
      );
      return;
    case "image":
      slide.addImage(imageOptions(element));
      return;
    case "table":
      return addTable(slide, element, authoring);
    case "chart":
      return addChart(slide, element, authoring);
    case "scatter":
      return addScatter(slide, element, authoring);
  }
}

// PptxGenJS cannot name a slide, so authored titles are written into each
// slide's cSld name after encoding.
function nameSlides(
  archive: OfficePackage,
  slides: PresentationContent["slides"],
): void {
  slides.forEach((slide, index) => {
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
}

export async function createPresentation(json: string): Promise<Uint8Array> {
  const content = parseContent(presentationContentSchema, json);
  const presentation = new PptxGenJS();
  const layout = LAYOUTS[content.layout ?? "wide"];
  const font = content.font ?? HOUSE_STYLE.font;
  presentation.layout = layout.name;
  presentation.author = content.author ?? HOUSE_STYLE.author;
  presentation.title = content.title;
  presentation.subject = content.subject ?? "";
  presentation.theme = { headFontFace: font, bodyFontFace: font };
  const authoring: Authoring = {
    presentation,
    font,
    language: content.language,
    layout,
  };
  const masters = defineMasters(content, authoring);
  for (const authored of content.slides) {
    if (authored.master && !masters.has(authored.master))
      throw new Error("slide references an undeclared master");
    const slide = presentation.addSlide(
      authored.master ? { masterName: authored.master } : undefined,
    );
    if (authored.background) slide.background = { color: authored.background };
    for (const element of authored.elements)
      addElement(slide, element, authoring);
    if (authored.notes) slide.addNotes(authored.notes);
  }
  const result = await presentation.write({
    outputType: "uint8array",
    compression: true,
  });
  if (!(result instanceof Uint8Array))
    throw new Error("presentation encoder returned an unexpected result");
  const archive = OfficePackage.open("pptx", result);
  nameSlides(archive, content.slides);
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
