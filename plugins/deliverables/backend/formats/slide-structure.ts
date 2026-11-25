import {
  OfficePackage,
  relations,
  relationshipsPath,
  sourceForRelationships,
  CONTENT_TYPES_PART,
  MAIN_PARTS,
} from "./archive";
import type { PresentationOperation } from "../../contracts/content";
import {
  child,
  placeholder,
  relativeTarget,
  sectionEntries,
  shapeTree,
  slideLayouts,
  slideList,
} from "./slides";
import {
  attribute,
  escapeXML,
  namespaces,
  openTag,
  type XmlNode,
  type XmlPatch,
} from "./xml";

// Relationship types whose targets belong to one slide; a duplicate receives
// its own copies. Layouts, media and links stay shared.
const OWNED = new Set([
  "notesSlide",
  "chart",
  "package",
  "chartUserShapes",
  "chartStyle",
  "chartColorStyle",
  "themeOverride",
  "comments",
  "diagramData",
  "diagramLayout",
  "diagramQuickStyle",
  "diagramColors",
  "diagramDrawing",
]);

function emptyTag(node: XmlNode, attributes: Record<string, string>): string {
  return `${openTag(node, attributes).slice(0, -1)}/>`;
}

export function reachableParts(archive: OfficePackage): Set<string> {
  const reached = new Set<string>();
  const pending = [""];
  while (pending.length)
    for (const relation of relations(archive, pending.pop()!))
      if (
        relation.target &&
        archive.parts.has(relation.target) &&
        !reached.has(relation.target)
      ) {
        reached.add(relation.target);
        pending.push(relation.target);
      }
  return reached;
}

// Removes only parts this edit disconnected; parts an imported package never
// reached remain untouched.
export function pruneParts(archive: OfficePackage, before: Set<string>): void {
  const after = reachableParts(archive);
  const removed = new Set([...before].filter((part) => !after.has(part)));
  if (!removed.size) return;
  for (const part of removed) {
    archive.parts.delete(part);
    archive.parts.delete(relationshipsPath(part));
  }
  const types = archive.xml(CONTENT_TYPES_PART);
  archive.patch(
    CONTENT_TYPES_PART,
    types.root.children
      .filter((node) => removed.has(node.attributes.PartName?.slice(1) ?? ""))
      .map((node) => ({ start: node.start, end: node.end, value: "" })),
  );
}

function freshPart(archive: OfficePackage, part: string): string {
  const match = /^(.*?)[0-9]*(\.[A-Za-z0-9]+)$/u.exec(part);
  if (!match) throw new Error("package part has no extension");
  const taken = new Set(
    [...archive.parts.keys()].map((name) => name.toLocaleLowerCase("en-US")),
  );
  for (let number = 1; ; number++) {
    const candidate = `${match[1]}${number}${match[2]}`;
    if (!taken.has(candidate.toLocaleLowerCase("en-US"))) return candidate;
  }
}

function copyContentType(
  archive: OfficePackage,
  source: string,
  copy: string,
): void {
  const types = archive.xml(CONTENT_TYPES_PART);
  const override = types.root.children.find(
    (node) =>
      node.local === "Override" && node.attributes.PartName === `/${source}`,
  );
  if (!override) return;
  archive.patch(CONTENT_TYPES_PART, [
    {
      start: override.end,
      end: override.end,
      value: emptyTag(override, {
        ...override.attributes,
        PartName: `/${copy}`,
      }),
    },
  ]);
}

function copyPart(
  archive: OfficePackage,
  part: string,
  copies: Map<string, string>,
): string {
  const existing = copies.get(part);
  if (existing) return existing;
  const copy = freshPart(archive, part);
  copies.set(part, copy);
  archive.parts.set(copy, archive.parts.get(part)!);
  copyContentType(archive, part, copy);
  const owned = relations(archive, part);
  if (!owned.length) return copy;
  const patches: XmlPatch[] = [];
  for (const relation of owned) {
    if (!relation.target) continue;
    const target =
      copies.get(relation.target) ??
      (OWNED.has(relation.type)
        ? copyPart(archive, relation.target, copies)
        : relation.target);
    patches.push({
      start: relation.node.start,
      end: relation.node.end,
      value: emptyTag(relation.node, {
        ...relation.node.attributes,
        Target: relativeTarget(copy, target),
      }),
    });
  }
  const source = archive.xml(relationshipsPath(part));
  archive.setXML(relationshipsPath(copy), source.source);
  archive.patch(relationshipsPath(copy), patches);
  return copy;
}

function nextRelationshipId(archive: OfficePackage, part: string): string {
  const used = new Set(relations(archive, part).map((relation) => relation.id));
  for (let number = used.size + 1; ; number++)
    if (!used.has(`rId${number}`)) return `rId${number}`;
}

// Lists a new slide part after another slide, or first; it joins that slide's
// sections.
function listSlide(
  archive: OfficePackage,
  part: string,
  after: string | undefined,
): void {
  const { document, slides } = slideList(archive);
  const anchor =
    after === undefined
      ? undefined
      : slides.find((entry) => entry.id === after)!;
  const template = anchor ?? slides[0]!;
  const relationship = nextRelationshipId(archive, MAIN_PARTS.pptx);
  const relationships = archive.xml("ppt/_rels/presentation.xml.rels");
  const relation = relations(archive, MAIN_PARTS.pptx).find(
    (candidate) => candidate.id === template.relation,
  )!.node;
  archive.patch("ppt/_rels/presentation.xml.rels", [
    {
      start: relationships.root.closeStart,
      end: relationships.root.closeStart,
      value: emptyTag(relation, {
        ...relation.attributes,
        Id: relationship,
        Target: relativeTarget(MAIN_PARTS.pptx, part),
      }),
    },
  ]);
  const used = new Set(slides.map((entry) => Number(entry.node.attributes.id)));
  let identity = Math.max(255, ...used) + 1;
  if (identity >= 2147483648)
    for (identity = 256; used.has(identity); identity++);
  const reference = Object.keys(template.node.attributes).find(
    (name) =>
      name !== "id" &&
      template.node.attributes[name] ===
        attribute(template.node, namespaces.officeRelationships, "id"),
  )!;
  const at = (node: XmlNode) => (anchor ? node.end : node.start);
  archive.patch(MAIN_PARTS.pptx, [
    {
      start: at(template.node),
      end: at(template.node),
      value: emptyTag(template.node, {
        id: String(identity),
        [reference]: relationship,
      }),
    },
    ...sectionEntries(document, template.node.attributes.id!).map((entry) => ({
      start: at(entry),
      end: at(entry),
      value: emptyTag(entry, { ...entry.attributes, id: String(identity) }),
    })),
  ]);
}

function duplicateSlide(archive: OfficePackage, slide: string): void {
  listSlide(archive, copyPart(archive, slide, new Map()), slide);
}

// A new slide takes its layout's title and body placeholders, as PowerPoint
// does, each with one empty run so replaceText can fill it; date, footer and
// number placeholders stay on the layout.
const INSTANTIATED = new Set(["title", "ctrTitle", "subTitle", "body", "obj"]);
const PLACEHOLDER_ATTRIBUTES = ["type", "orient", "sz", "idx"];

function insertSlide(
  archive: OfficePackage,
  layout: string,
  after: string | undefined,
): string {
  const { slides } = slideList(archive);
  const part = freshPart(archive, "ppt/slides/slide1.xml");
  let identity = 1;
  const shapes = shapeTree(archive.xml(layout)).children.flatMap((node) => {
    const kind = placeholder(node)?.type;
    if (node.local !== "sp" || !kind || !INSTANTIATED.has(kind)) return [];
    const properties = child(node, namespaces.presentation, "nvSpPr");
    const ph = child(
      child(properties, namespaces.presentation, "nvPr"),
      namespaces.presentation,
      "ph",
    )!;
    const name = child(properties, namespaces.presentation, "cNvPr")?.attributes
      .name;
    const attributes = PLACEHOLDER_ATTRIBUTES.flatMap((key) =>
      ph.attributes[key] === undefined
        ? []
        : [` ${key}="${escapeXML(ph.attributes[key])}"`],
    ).join("");
    identity++;
    return [
      `<p:sp><p:nvSpPr><p:cNvPr id="${identity}" name="${escapeXML(name ?? `Placeholder ${identity}`)}"/><p:cNvSpPr><a:spLocks noGrp="1"/></p:cNvSpPr><p:nvPr><p:ph${attributes}/></p:nvPr></p:nvSpPr><p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/><a:p><a:r><a:t></a:t></a:r></a:p></p:txBody></p:sp>`,
    ];
  });
  archive.setXML(
    part,
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n<p:sld xmlns:a="${namespaces.drawing}" xmlns:r="${namespaces.officeRelationships}" xmlns:p="${namespaces.presentation}"><p:cSld><p:spTree><p:nvGrpSpPr><p:cNvPr id="1" name=""/><p:cNvGrpSpPr/><p:nvPr/></p:nvGrpSpPr><p:grpSpPr><a:xfrm><a:off x="0" y="0"/><a:ext cx="0" cy="0"/><a:chOff x="0" y="0"/><a:chExt cx="0" cy="0"/></a:xfrm></p:grpSpPr>${shapes.join("")}</p:spTree></p:cSld><p:clrMapOvr><a:masterClrMapping/></p:clrMapOvr></p:sld>`,
  );
  archive.setXML(
    relationshipsPath(part),
    `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n<Relationships xmlns="${namespaces.relationships}"><Relationship Id="rId1" Type="${namespaces.officeRelationships}/slideLayout" Target="${relativeTarget(part, layout)}"/></Relationships>`,
  );
  copyContentType(archive, slides[0]!.id, part);
  listSlide(archive, part, after);
  return part;
}

function incomingLinks(archive: OfficePackage, slide: string, notes?: string) {
  return [...archive.parts.keys()]
    .filter((part) => /(?:^|\/)_rels\/[^/]+\.rels$/u.test(part))
    .flatMap((path) => {
      const source = sourceForRelationships(path);
      if (source === MAIN_PARTS.pptx || source === notes) return [];
      return relations(archive, source).filter(
        (relation) => relation.target === slide,
      );
    });
}

function deleteSlide(archive: OfficePackage, slide: string): void {
  const { document, slides } = slideList(archive);
  const entry = slides.find((candidate) => candidate.id === slide)!;
  const references = document.nodes.filter((node) =>
    Object.entries(node.qualifiedAttributes).some(
      ([name, value]) =>
        name.startsWith(`${namespaces.officeRelationships}|`) &&
        value === entry.relation,
    ),
  );
  if (references.length !== 1)
    throw new Error("a custom show uses this slide; remove it there first");
  const notes = relations(archive, slide).find(
    (relation) => relation.type === "notesSlide",
  )?.target;
  if (incomingLinks(archive, slide, notes).length)
    throw new Error("another part links to this slide; remove that link first");
  archive.patch(MAIN_PARTS.pptx, [
    { start: entry.node.start, end: entry.node.end, value: "" },
    ...sectionEntries(document, entry.node.attributes.id!).map((node) => ({
      start: node.start,
      end: node.end,
      value: "",
    })),
  ]);
  const relation = relations(archive, MAIN_PARTS.pptx).find(
    (candidate) => candidate.id === entry.relation,
  )!;
  archive.patch("ppt/_rels/presentation.xml.rels", [
    { start: relation.node.start, end: relation.node.end, value: "" },
  ]);
}

type Restructure = Extract<
  PresentationOperation,
  { type: "duplicateSlide" | "deleteSlide" | "insertSlide" }
>;

export function restructureSlides(
  archive: OfficePackage,
  operations: Restructure[],
): void {
  const targets = operations.flatMap((operation) =>
    operation.type === "insertSlide" ? [] : [operation.id],
  );
  if (new Set(targets).size !== targets.length)
    throw new Error(
      "one slide may be duplicated or deleted only once per revision",
    );
  const { slides } = slideList(archive);
  const known = (id: string) => slides.some((slide) => slide.id === id);
  const layouts = new Set(slideLayouts(archive).map((layout) => layout.id));
  for (const operation of operations)
    if (operation.type === "insertSlide") {
      if (operation.after !== undefined && !known(operation.after))
        throw new Error(
          "slide identifier does not belong to this presentation",
        );
      if (!layouts.has(operation.layout))
        throw new Error("slide layout does not belong to this presentation");
    } else if (!known(operation.id))
      throw new Error("slide identifier does not belong to this presentation");
  const deletions = operations.filter(
    (operation) => operation.type === "deleteSlide",
  );
  if (deletions.length >= slides.length)
    throw new Error("a presentation must retain at least one slide");
  const before = reachableParts(archive);
  // Slides inserted after the same slide keep their operation order.
  const inserted = new Map<string | undefined, string>();
  for (const operation of operations)
    if (operation.type === "duplicateSlide")
      duplicateSlide(archive, operation.id);
    else if (operation.type === "insertSlide")
      inserted.set(
        operation.after,
        insertSlide(
          archive,
          operation.layout,
          inserted.get(operation.after) ?? operation.after,
        ),
      );
  for (const operation of deletions) deleteSlide(archive, operation.id);
  pruneParts(archive, before);
}
