import {
  documentEditSchema,
  parseContent,
  type DocumentOperation,
} from "../../contracts/content";
import { MAIN_PARTS, OfficePackage } from "./archive";
import { bodyBlocks, documentBody, type BodyBlock } from "./document-blocks";
import { textPatches } from "./text-edits";
import {
  attribute,
  escapeXML,
  namespaces,
  tagName,
  type XmlDocument,
  type XmlNode,
  type XmlPatch,
} from "./xml";

type Of<T extends DocumentOperation["type"]> = Extract<
  DocumentOperation,
  { type: T }
>;

const isWord = (node: XmlNode, local: string) =>
  node.uri === namespaces.word && node.local === local;

function assertStylesDefined(archive: OfficePackage, styles: Set<string>) {
  styles.delete("Normal");
  if (!styles.size) return;
  const defined = new Set(
    archive.parts.has("word/styles.xml")
      ? archive
          .xml("word/styles.xml")
          .nodes.filter(
            (node) =>
              isWord(node, "style") &&
              attribute(node, namespaces.word, "type") === "paragraph",
          )
          .map((node) => attribute(node, namespaces.word, "styleId"))
      : [],
  );
  for (const style of styles)
    if (!defined.has(style))
      throw new Error(
        `paragraph style ${style} is not defined in this document`,
      );
}

function paragraphMarkup(
  body: XmlNode,
  text: string,
  style: string | undefined,
): string {
  const p = tagName(body, "p");
  const pPr = tagName(body, "pPr");
  const pStyle = tagName(body, "pStyle");
  const r = tagName(body, "r");
  const t = tagName(body, "t");
  const val = tagName(body, "val");
  // An empty run keeps the new paragraph's text editable with replaceText.
  return `<${p}>${style ? `<${pPr}><${pStyle} ${val}="${style}"/></${pPr}>` : ""}<${r}><${t} xml:space="preserve">${escapeXML(text)}</${t}></${r}></${p}>`;
}

function stylePatch(paragraph: XmlNode, style: string): XmlPatch[] {
  const pPr = paragraph.children.find((node) => isWord(node, "pPr"));
  const pStyle = pPr?.children.find((node) => isWord(node, "pStyle"));
  const markup = `<${tagName(paragraph, "pStyle")} ${tagName(paragraph, "val")}="${style}"/>`;
  if (pStyle)
    return [
      {
        start: pStyle.start,
        end: pStyle.end,
        value: style === "Normal" ? "" : markup,
      },
    ];
  if (style === "Normal") return [];
  if (pPr && !pPr.selfClosing)
    return [{ start: pPr.openEnd, end: pPr.openEnd, value: markup }];
  const properties = `<${tagName(paragraph, "pPr")}>${markup}</${tagName(paragraph, "pPr")}>`;
  if (pPr) return [{ start: pPr.start, end: pPr.end, value: properties }];
  if (paragraph.selfClosing)
    return [
      {
        start: paragraph.start,
        end: paragraph.end,
        value: `<${paragraph.name}>${properties}</${paragraph.name}>`,
      },
    ];
  return [
    { start: paragraph.openEnd, end: paragraph.openEnd, value: properties },
  ];
}

// Every block operation names the block it read by position and exact XML, so
// a block that changed or moved since that read is refused.
function blockPatches(
  document: XmlDocument,
  operations: DocumentOperation[],
): XmlPatch[] {
  const body = documentBody(document);
  const blocks = new Map(
    bodyBlocks(document).map((block) => [block.id, block]),
  );
  const target = (id: string, expect: string): BodyBlock => {
    const block = blocks.get(id);
    if (!block)
      throw new Error("block identifier does not belong to this document");
    if (block.expect !== expect)
      throw new Error("block precondition does not match the exact revision");
    return block;
  };
  const section = body.children.find((node) => isWord(node, "sectPr"));
  // Paragraphs inserted at one position keep their operation order.
  const inserted = new Map<number, string[]>();
  const insert = (position: number, text: string, style?: string) =>
    inserted.set(position, [
      ...(inserted.get(position) ?? []),
      paragraphMarkup(body, text, style),
    ]);
  const patches: XmlPatch[] = [];
  for (const operation of operations) {
    switch (operation.type) {
      case "appendParagraph":
        insert(
          section?.start ?? body.closeStart,
          operation.text,
          operation.style,
        );
        break;
      case "insertParagraph":
        insert(
          operation.after === undefined
            ? body.openEnd
            : target(operation.after, operation.expect!).node.end,
          operation.text,
          operation.style,
        );
        break;
      case "deleteBlock": {
        const { node } = target(operation.id, operation.expect);
        patches.push({ start: node.start, end: node.end, value: "" });
        break;
      }
      case "setParagraphStyle": {
        const { node } = target(operation.id, operation.expect);
        if (!isWord(node, "p"))
          throw new Error("paragraph styles apply only to paragraphs");
        patches.push(...stylePatch(node, operation.style));
        break;
      }
    }
  }
  for (const [position, markup] of inserted)
    patches.push({ start: position, end: position, value: markup.join("") });
  return patches;
}

export function editDocument(archive: OfficePackage, json: string): Uint8Array {
  const { operations } = parseContent(documentEditSchema, json);
  assertStylesDefined(
    archive,
    new Set(
      operations.flatMap((operation) =>
        "style" in operation && operation.style ? [operation.style] : [],
      ),
    ),
  );
  const edits = textPatches(
    archive,
    operations.filter(
      (operation): operation is Of<"replaceText"> =>
        operation.type === "replaceText",
    ),
  );
  const document = archive.xml(MAIN_PARTS.docx);
  // One patch set per part, so a block edit overlapping a text edit is refused.
  edits.set(MAIN_PARTS.docx, [
    ...(edits.get(MAIN_PARTS.docx) ?? []),
    ...blockPatches(
      document,
      operations.filter((operation) => operation.type !== "replaceText"),
    ),
  ]);
  for (const [part, patches] of edits) archive.patch(part, patches);
  return archive.save();
}
