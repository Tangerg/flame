import PptxGenJS from "pptxgenjs";
import {
  parseContent,
  presentationContentSchema,
  presentationEditSchema,
  type Inspection,
  type SlideElement,
} from "../../contracts/content";
import { OfficePackage, relationshipTarget } from "./archive";
import { imageBytes } from "./images";
import { inspectOfficeText, officeReport } from "./inspection";
import { assertPermutation, replaceText } from "./text-edits";
import { attribute, namespaces, openTag, patchXML } from "./xml";

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
          if (
            element.chart === "scatter" &&
            element.series.some((series) =>
              series.labels.some((label) => !Number.isFinite(Number(label))),
            )
          )
            throw new Error(
              "scatter chart labels must contain numeric x values",
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
              chartColors: ["245B78", "39A597", "DF9D3A", "8B6FB3"],
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

function slideList(archive: OfficePackage) {
  const document = archive.xml("ppt/presentation.xml");
  const list = document.root.children.find(
    (node) => node.uri === namespaces.presentation && node.local === "sldIdLst",
  );
  const relationships = archive.xml("ppt/_rels/presentation.xml.rels").root
    .children;
  if (!list) throw new Error("presentation has no slide list");
  const slides = list.children.map((node) => {
    const relation = relationships.find(
      (relation) =>
        relation.attributes.Id ===
        attribute(node, namespaces.officeRelationships, "id"),
    );
    if (
      node.uri !== namespaces.presentation ||
      node.local !== "sldId" ||
      !relation?.attributes.Type?.endsWith("/slide")
    )
      throw new Error("invalid presentation slide relationship");
    return {
      node,
      id: relationshipTarget(
        "ppt/presentation.xml",
        relation.attributes.Target!,
      ),
    };
  });
  if (new Set(slides.map((slide) => slide.id)).size !== slides.length)
    throw new Error("presentation contains duplicate slide references");
  return { document, list, slides };
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

export function editPresentation(
  archive: OfficePackage,
  json: string,
): Uint8Array {
  const { operations } = parseContent(presentationEditSchema, json);
  replaceText(
    archive,
    operations.filter((operation) => operation.type === "replaceText"),
  );
  const reorders = operations.filter(
    (operation) => operation.type === "reorderSlides",
  );
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
    archive.setXML(
      "ppt/presentation.xml",
      patchXML(document.source, [
        {
          start: list.openEnd,
          end: list.closeStart,
          value: reorders[0].ids.map((id) => values.get(id)!).join(""),
        },
      ]),
    );
  }
  return archive.save();
}
