import {
  OfficePackage,
  relations,
  relationshipsPath,
  sourceForRelationships,
} from "./archive";
import { relativeTarget, sectionEntries, slideList } from "./slides";
import { attribute, namespaces, openTag, type XmlNode, type XmlPatch } from "./xml";

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
  const types = archive.xml("[Content_Types].xml");
  archive.patch(
    "[Content_Types].xml",
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
  const types = archive.xml("[Content_Types].xml");
  const override = types.root.children.find(
    (node) => node.local === "Override" && node.attributes.PartName === `/${source}`,
  );
  if (!override) return;
  archive.patch("[Content_Types].xml", [
    {
      start: override.end,
      end: override.end,
      value: emptyTag(override, { ...override.attributes, PartName: `/${copy}` }),
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

function duplicateSlide(archive: OfficePackage, slide: string): void {
  const { document, slides } = slideList(archive);
  const source = slides.find((entry) => entry.id === slide)!;
  const copy = copyPart(archive, slide, new Map());
  const relationship = nextRelationshipId(archive, "ppt/presentation.xml");
  const relationships = archive.xml("ppt/_rels/presentation.xml.rels");
  const template = relations(archive, "ppt/presentation.xml").find(
    (relation) => relation.id === source.relation,
  )!.node;
  archive.patch("ppt/_rels/presentation.xml.rels", [
    {
      start: relationships.root.closeStart,
      end: relationships.root.closeStart,
      value: emptyTag(template, {
        ...template.attributes,
        Id: relationship,
        Target: relativeTarget("ppt/presentation.xml", copy),
      }),
    },
  ]);
  const used = new Set(slides.map((entry) => Number(entry.node.attributes.id)));
  let identity = Math.max(255, ...used) + 1;
  if (identity >= 2147483648)
    for (identity = 256; used.has(identity); identity++);
  const reference = Object.keys(source.node.attributes).find(
    (name) =>
      name !== "id" &&
      source.node.attributes[name] ===
        attribute(source.node, namespaces.officeRelationships, "id"),
  )!;
  archive.patch("ppt/presentation.xml", [
    {
      start: source.node.end,
      end: source.node.end,
      value: emptyTag(source.node, {
        id: String(identity),
        [reference]: relationship,
      }),
    },
    ...sectionEntries(document, source.node.attributes.id!).map((entry) => ({
      start: entry.end,
      end: entry.end,
      value: emptyTag(entry, { ...entry.attributes, id: String(identity) }),
    })),
  ]);
}

function incomingLinks(archive: OfficePackage, slide: string, notes?: string) {
  return [...archive.parts.keys()]
    .filter((part) => /(?:^|\/)_rels\/[^/]+\.rels$/u.test(part))
    .flatMap((path) => {
      const source = sourceForRelationships(path);
      if (source === "ppt/presentation.xml" || source === notes) return [];
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
  archive.patch("ppt/presentation.xml", [
    { start: entry.node.start, end: entry.node.end, value: "" },
    ...sectionEntries(document, entry.node.attributes.id!).map((node) => ({
      start: node.start,
      end: node.end,
      value: "",
    })),
  ]);
  const relation = relations(archive, "ppt/presentation.xml").find(
    (candidate) => candidate.id === entry.relation,
  )!;
  archive.patch("ppt/_rels/presentation.xml.rels", [
    { start: relation.node.start, end: relation.node.end, value: "" },
  ]);
}

export function restructureSlides(
  archive: OfficePackage,
  operations: Array<{ type: "duplicateSlide" | "deleteSlide"; id: string }>,
): void {
  const targets = operations.map((operation) => operation.id);
  if (new Set(targets).size !== targets.length)
    throw new Error("one slide may be duplicated or deleted only once per revision");
  const { slides } = slideList(archive);
  if (targets.some((id) => !slides.some((slide) => slide.id === id)))
    throw new Error("slide identifier does not belong to this presentation");
  const deletions = operations.filter((operation) => operation.type === "deleteSlide");
  if (deletions.length >= slides.length)
    throw new Error("a presentation must retain at least one slide");
  const before = reachableParts(archive);
  for (const operation of operations)
    if (operation.type === "duplicateSlide") duplicateSlide(archive, operation.id);
  for (const operation of deletions) deleteSlide(archive, operation.id);
  pruneParts(archive, before);
}
