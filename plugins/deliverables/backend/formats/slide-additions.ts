import type { PresentationOperation } from "../../contracts/content";
import { OfficePackage } from "./archive";
import { child, shapeTree, slideList } from "./slides";
import {
  escapeXML,
  namespaces,
  tagName,
  type XmlDocument,
  type XmlPatch,
} from "./xml";

type Addition = Extract<
  PresentationOperation,
  { type: "addText" | "addShape" }
>;

// Theme-referenced styling, so an added shape follows the deck's colours the
// way PowerPoint's own inserted shapes do.
const SHAPE_STYLE =
  '<a:lnRef idx="2"><a:schemeClr val="accent1"><a:shade val="50000"/></a:schemeClr></a:lnRef><a:fillRef idx="1"><a:schemeClr val="accent1"/></a:fillRef><a:effectRef idx="0"><a:schemeClr val="accent1"/></a:effectRef><a:fontRef idx="minor"><a:schemeClr val="lt1"/></a:fontRef>';

function drawingPrefix(document: XmlDocument) {
  const declared = Object.entries(document.root.attributes).find(
    ([name, value]) =>
      name.startsWith("xmlns:") && value === namespaces.drawing,
  )?.[0];
  return declared
    ? { prefix: declared.slice(6), declaration: "" }
    : { prefix: "a", declaration: ` xmlns:a="${namespaces.drawing}"` };
}

function paragraphs(text: string, alignment: string): string {
  return text
    .split("\n")
    .map(
      (line) =>
        `<a:p>${alignment}${line ? `<a:r><a:t>${escapeXML(line)}</a:t></a:r>` : "<a:endParaRPr/>"}</a:p>`,
    )
    .join("");
}

function markup(operation: Addition, id: number): string {
  const { x, y, width, height } = operation.frame;
  const frame = `<a:xfrm><a:off x="${x}" y="${y}"/><a:ext cx="${width}" cy="${height}"/></a:xfrm>`;
  if (operation.type === "addText")
    return `<p:sp><p:nvSpPr><p:cNvPr id="${id}" name="TextBox ${id}"/><p:cNvSpPr txBox="1"/><p:nvPr/></p:nvSpPr><p:spPr>${frame}<a:prstGeom prst="rect"><a:avLst/></a:prstGeom><a:noFill/></p:spPr><p:txBody><a:bodyPr wrap="square" rtlCol="0"/><a:lstStyle/>${paragraphs(operation.text, "")}</p:txBody></p:sp>`;
  const fill = operation.fill
    ? `<a:solidFill><a:srgbClr val="${operation.fill.toUpperCase()}"/></a:solidFill>`
    : "";
  const line = operation.line
    ? `<a:ln><a:solidFill><a:srgbClr val="${operation.line.toUpperCase()}"/></a:solidFill></a:ln>`
    : "";
  const body = operation.text
    ? paragraphs(operation.text, '<a:pPr algn="ctr"/>')
    : '<a:p><a:pPr algn="ctr"/><a:endParaRPr/></a:p>';
  return `<p:sp><p:nvSpPr><p:cNvPr id="${id}" name="Shape ${id}"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr><p:spPr>${frame}<a:prstGeom prst="${operation.shape}"><a:avLst/></a:prstGeom>${fill}${line}</p:spPr><p:style>${SHAPE_STYLE}</p:style><p:txBody><a:bodyPr rtlCol="0" anchor="ctr"/><a:lstStyle/>${body}</p:txBody></p:sp>`;
}

// The markup is written with the p: and a: prefixes and then renamed to the
// slide's own, so a slide that spells its namespaces differently stays valid.
function inSlidePrefixes(value: string, document: XmlDocument): string {
  const tree = shapeTree(document);
  const presentation = tagName(tree, "");
  const drawing = drawingPrefix(document);
  return value
    .replace(/<(\/?)p:/gu, `<$1${presentation}`)
    .replace(/<(\/?)a:/gu, `<$1${drawing.prefix}:`)
    .replace(/^<([^ >]+)/u, `<$1${drawing.declaration}`);
}

// Added elements go after the slide's existing ones, so every text run already
// on the slide keeps its identity. Shape identities continue the slide's
// highest one, as PowerPoint requires them unique within a slide.
export function additionPatches(
  archive: OfficePackage,
  operations: Addition[],
): Map<string, XmlPatch> {
  const slides = new Set(slideList(archive).slides.map((slide) => slide.id));
  const bySlide = Map.groupBy(operations, (operation) => operation.slide);
  const patches = new Map<string, XmlPatch>();
  for (const [part, added] of bySlide) {
    if (!slides.has(part))
      throw new Error("slide identifier does not belong to this presentation");
    const document = archive.xml(part);
    let identity = Math.max(
      0,
      ...document.nodes
        .filter(
          (node) =>
            node.uri === namespaces.presentation &&
            node.local === "cNvPr" &&
            /^[0-9]+$/u.test(node.attributes.id ?? ""),
        )
        .map((node) => Number(node.attributes.id)),
    );
    const tree = shapeTree(document);
    const at =
      child(tree, namespaces.presentation, "extLst")?.start ?? tree.closeStart;
    patches.set(part, {
      start: at,
      end: at,
      value: added
        .map((operation) =>
          inSlidePrefixes(markup(operation, ++identity), document),
        )
        .join(""),
    });
  }
  return patches;
}
