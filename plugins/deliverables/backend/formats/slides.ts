import {
  slideLayoutSchema,
  type ElementView,
  type Frame,
  type SlideLayout,
} from "../../contracts/content";
import { OfficePackage, relations, relationshipTarget } from "./archive";
import { readChart } from "./charts";
import { textNodes } from "./inspection";
import { attribute, namespaces, type XmlDocument, type XmlNode } from "./xml";

const POWERPOINT_2010 = "http://schemas.microsoft.com/office/powerpoint/2010/main";

export function child(
  node: XmlNode | undefined,
  uri: string,
  local: string,
): XmlNode | undefined {
  return node?.children.find(
    (entry) => entry.uri === uri && entry.local === local,
  );
}

export function slideList(archive: OfficePackage) {
  const document = archive.xml("ppt/presentation.xml");
  const list = child(document.root, namespaces.presentation, "sldIdLst");
  const presentationRelations = relations(archive, "ppt/presentation.xml");
  if (!list) throw new Error("presentation has no slide list");
  const slides = list.children.map((node) => {
    const relation = presentationRelations.find(
      (entry) =>
        entry.id === attribute(node, namespaces.officeRelationships, "id"),
    );
    if (
      node.uri !== namespaces.presentation ||
      node.local !== "sldId" ||
      relation?.type !== "slide" ||
      !relation.target
    )
      throw new Error("invalid presentation slide relationship");
    return { node, id: relation.target, relation: relation.id };
  });
  if (new Set(slides.map((slide) => slide.id)).size !== slides.length)
    throw new Error("presentation contains duplicate slide references");
  return { document, list, slides };
}

// Section membership lists slide identities in PowerPoint's 2010 extension.
export function sectionEntries(document: XmlDocument, slideId: string) {
  return document.nodes.filter(
    (node) =>
      node.uri === POWERPOINT_2010 &&
      node.local === "sldId" &&
      node.attributes.id === slideId,
  );
}

function shapeTree(document: XmlDocument): XmlNode {
  const tree = child(
    child(document.root, namespaces.presentation, "cSld"),
    namespaces.presentation,
    "spTree",
  );
  if (!tree) throw new Error("slide has no shape tree");
  return tree;
}

function nonVisual(node: XmlNode): XmlNode | undefined {
  for (const entry of node.children) {
    if (entry.uri === namespaces.presentation && entry.local === "cNvPr")
      return entry;
    const found = nonVisual(entry);
    if (found) return found;
  }
}

export interface SlideElement {
  id: string;
  node: XmlNode;
  name: string;
}

export function slideElements(
  part: string,
  document: XmlDocument,
): SlideElement[] {
  return shapeTree(document).children.flatMap((node) => {
    if (
      node.uri === namespaces.presentation &&
      ["nvGrpSpPr", "grpSpPr", "extLst"].includes(node.local)
    )
      return [];
    const identity = nonVisual(node);
    if (!identity || !/^[0-9]+$/u.test(identity.attributes.id ?? ""))
      return [];
    return [
      {
        id: `${part}#element:${identity.attributes.id}`,
        node,
        name: identity.attributes.name ?? "",
      },
    ];
  });
}

export function resolveElement(archive: OfficePackage, id: string) {
  const separator = id.lastIndexOf("#element:");
  const part = id.slice(0, separator);
  if (
    separator < 0 ||
    !slideList(archive).slides.some((slide) => slide.id === part)
  )
    throw new Error(
      "slide element identifier does not belong to this presentation",
    );
  const document = archive.xml(part);
  const matches = slideElements(part, document).filter(
    (element) => element.id === id,
  );
  if (matches.length !== 1)
    throw new Error(
      matches.length
        ? "slide element identifier is ambiguous in this revision"
        : "slide element does not exist in this revision",
    );
  return { part, document, element: matches[0]! };
}

// The element's own property container: xfrm lives here when it is explicit.
export function propertiesNode(node: XmlNode): XmlNode | undefined {
  switch (node.local) {
    case "sp":
    case "pic":
    case "cxnSp":
      return child(node, namespaces.presentation, "spPr");
    case "grpSp":
      return child(node, namespaces.presentation, "grpSpPr");
  }
}

export function transformNode(node: XmlNode): XmlNode | undefined {
  if (node.local === "graphicFrame")
    return child(node, namespaces.presentation, "xfrm");
  return child(propertiesNode(node), namespaces.drawing, "xfrm");
}

export function frameOf(transform: XmlNode | undefined): Frame | undefined {
  const offset = child(transform, namespaces.drawing, "off");
  const extent = child(transform, namespaces.drawing, "ext");
  if (!offset || !extent) return;
  const values = [
    offset.attributes.x,
    offset.attributes.y,
    extent.attributes.cx,
    extent.attributes.cy,
  ];
  if (values.some((value) => !/^-?[0-9]{1,15}$/u.test(value ?? ""))) return;
  const [x, y, width, height] = values.map(Number) as [
    number,
    number,
    number,
    number,
  ];
  return { x, y, width, height };
}

function placeholder(node: XmlNode) {
  const properties = node.children.find(
    (entry) =>
      entry.uri === namespaces.presentation && entry.local.startsWith("nv"),
  );
  const ph = child(
    child(properties, namespaces.presentation, "nvPr"),
    namespaces.presentation,
    "ph",
  );
  if (!ph) return;
  return { type: ph.attributes.type ?? "obj", index: ph.attributes.idx };
}

function masterType(type: string): string {
  if (type === "ctrTitle") return "title";
  if (type === "subTitle" || type === "obj") return "body";
  return type;
}

// PowerPoint resolves a layout placeholder by index first, then by type; the
// master holds only the typed title, body and footer placeholders.
function matchPlaceholder(
  document: XmlDocument,
  wanted: NonNullable<ReturnType<typeof placeholder>>,
  master: boolean,
): XmlNode | undefined {
  const candidates = shapeTree(document).children.filter((node) =>
    placeholder(node),
  );
  if (!master && wanted.index !== undefined) {
    const indexed = candidates.find(
      (node) => placeholder(node)!.index === wanted.index,
    );
    if (indexed) return indexed;
  }
  const type = master ? masterType(wanted.type) : wanted.type;
  return candidates.find((node) => {
    const candidate = placeholder(node)!.type;
    return (master ? masterType(candidate) : candidate) === type;
  });
}

export function elementFrame(
  archive: OfficePackage,
  slidePart: string,
  node: XmlNode,
): { frame: Frame; source: "slide" | "layout" | "master" } | undefined {
  const own = frameOf(transformNode(node));
  if (own) return { frame: own, source: "slide" };
  const wanted = placeholder(node);
  if (!wanted) return;
  const layout = relations(archive, slidePart).find(
    (relation) => relation.type === "slideLayout",
  )?.target;
  if (!layout) return;
  const fromLayout = matchPlaceholder(archive.xml(layout), wanted, false);
  const layoutFrame = fromLayout && frameOf(transformNode(fromLayout));
  if (layoutFrame) return { frame: layoutFrame, source: "layout" };
  const master = relations(archive, layout).find(
    (relation) => relation.type === "slideMaster",
  )?.target;
  if (!master) return;
  const fromMaster = matchPlaceholder(
    archive.xml(master),
    (fromLayout && placeholder(fromLayout)) ?? wanted,
    true,
  );
  const masterFrame = fromMaster && frameOf(transformNode(fromMaster));
  if (masterFrame) return { frame: masterFrame, source: "master" };
}

export function chartPart(
  archive: OfficePackage,
  slidePart: string,
  node: XmlNode,
): string | undefined {
  if (node.local !== "graphicFrame") return;
  const reference = child(
    child(
      child(node, namespaces.drawing, "graphic"),
      namespaces.drawing,
      "graphicData",
    ),
    namespaces.chart,
    "chart",
  );
  const id = reference && attribute(reference, namespaces.officeRelationships, "id");
  if (!id) return;
  const relation = relations(archive, slidePart).find(
    (entry) => entry.id === id,
  );
  if (relation?.type !== "chart" || !relation.target)
    throw new Error("slide chart relationship is invalid");
  return relation.target;
}

function elementKind(node: XmlNode, hasText: boolean): ElementView["kind"] {
  switch (node.local) {
    case "sp":
      return hasText ? "text" : "shape";
    case "pic":
      return "picture";
    case "grpSp":
      return "group";
    case "cxnSp":
      return "connector";
    case "graphicFrame": {
      const uri = child(
        child(node, namespaces.drawing, "graphic"),
        namespaces.drawing,
        "graphicData",
      )?.attributes.uri;
      if (uri === namespaces.chart) return "chart";
      if (uri === "http://schemas.openxmlformats.org/drawingml/2006/table")
        return "table";
    }
  }
  return "other";
}

function runsWithin(
  part: string,
  runs: XmlNode[],
  node: XmlNode,
): ElementView["text"] {
  return runs.flatMap((run, index) =>
    run.start >= node.start && run.end <= node.end && !run.children.length
      ? [{ id: `${part}#text:${index}`, text: run.text }]
      : [],
  );
}

function notes(archive: OfficePackage, slidePart: string) {
  const part = relations(archive, slidePart).find(
    (relation) => relation.type === "notesSlide",
  )?.target;
  if (!part) return [];
  const { document, nodes } = textNodes(archive, part);
  return shapeTree(document).children.flatMap((node) =>
    placeholder(node)?.type === "body" ? runsWithin(part, nodes, node) : [],
  );
}

export function slideLayout(archive: OfficePackage): SlideLayout {
  const size = child(
    archive.xml("ppt/presentation.xml").root,
    namespaces.presentation,
    "sldSz",
  );
  const slides = slideList(archive).slides.map((slide, index) => {
    const { document, nodes } = textNodes(archive, slide.id);
    const elements = slideElements(slide.id, document).map(
      ({ id, node, name }): ElementView => {
        const text = runsWithin(slide.id, nodes, node);
        const placed = elementFrame(archive, slide.id, node);
        const rotation = transformNode(node)?.attributes.rot;
        const part = chartPart(archive, slide.id, node);
        let chart: ElementView["chart"];
        if (part)
          try {
            const model = readChart(archive, part);
            chart = {
              type: model.type,
              workbook: model.workbook.kind,
              data: model.data,
            };
          } catch {
            // Unsupported chart structures stay visible but expose no editable data.
          }
        return {
          id,
          kind: elementKind(node, text.length > 0),
          name,
          ...(placed ? { frame: placed.frame, frameSource: placed.source } : {}),
          ...(rotation && /^-?[0-9]+$/u.test(rotation)
            ? { rotation: Number(rotation) / 60000 }
            : {}),
          text,
          ...(chart ? { chart } : {}),
        };
      },
    );
    return {
      id: slide.id,
      number: index + 1,
      name: child(document.root, namespaces.presentation, "cSld")?.attributes.name ?? "",
      hidden: document.root.attributes.show === "0",
      elements,
      notes: notes(archive, slide.id),
    };
  });
  return slideLayoutSchema.parse({
    width: Number(size?.attributes.cx),
    height: Number(size?.attributes.cy),
    slides,
  });
}

export function relativeTarget(source: string, target: string): string {
  const from = source.split("/").slice(0, -1);
  const to = target.split("/");
  let shared = 0;
  while (shared < from.length && from[shared] === to[shared]) shared++;
  const result = [
    ...from.slice(shared).map(() => ".."),
    ...to.slice(shared),
  ].join("/");
  if (relationshipTarget(source, result) !== target)
    throw new Error("package relationship target cannot be expressed");
  return result;
}
