import { createHash } from "node:crypto";
import type {
  BlockView,
  DocumentLayout,
  ParagraphView,
} from "../../contracts/layouts";
import { MAIN_PARTS, OfficePackage } from "./archive";
import { textNodes, textRunId } from "./inspection";
import { attribute, namespaces, type XmlDocument, type XmlNode } from "./xml";

const isWord = (node: XmlNode, local: string) =>
  node.uri === namespaces.word && node.local === local;

const valueOf = (node: XmlNode | undefined) =>
  node && attribute(node, namespaces.word, "val");

// Range markers and section properties sit between blocks without content.
const MARKERS = new Set([
  "sectPr",
  "bookmarkStart",
  "bookmarkEnd",
  "commentRangeStart",
  "commentRangeEnd",
  "permStart",
  "permEnd",
  "proofErr",
]);

export interface BodyBlock {
  id: string;
  expect: string;
  node: XmlNode;
}

export function documentBody(document: XmlDocument): XmlNode {
  const body = document.root.children.find((node) => isWord(node, "body"));
  if (!body || body.selfClosing)
    throw new Error("document has no editable body");
  return body;
}

// Block identities are positions in the body of one exact revision, like the
// text run identities they contain.
export function bodyBlocks(document: XmlDocument): BodyBlock[] {
  return documentBody(document)
    .children.filter(
      (node) => !(node.uri === namespaces.word && MARKERS.has(node.local)),
    )
    .map((node, index) => ({
      id: `${MAIN_PARTS.docx}#block:${index}`,
      expect: createHash("sha256")
        .update(document.source.slice(node.start, node.end))
        .digest("hex"),
      node,
    }));
}

function switchedOn(properties: XmlNode | undefined, local: string): boolean {
  const toggle = properties?.children.find((node) => isWord(node, local));
  return (
    !!toggle && !["0", "false", "off", "none"].includes(valueOf(toggle) ?? "")
  );
}

// Text boxes and alternate content carry their own paragraphs, so their runs
// are not part of the enclosing paragraph's text.
function paragraphRuns(paragraph: XmlNode, index: Map<XmlNode, number>) {
  const runs: ParagraphView["runs"] = [];
  const visit = (node: XmlNode, run: XmlNode | undefined) => {
    if (isWord(node, "txbxContent") || node.local === "AlternateContent")
      return;
    if (isWord(node, "t")) {
      const properties = run?.children.find((child) => isWord(child, "rPr"));
      runs.push({
        id: textRunId(MAIN_PARTS.docx, index.get(node)!),
        text: node.text,
        bold: switchedOn(properties, "b"),
        italic: switchedOn(properties, "i"),
        underline: switchedOn(properties, "u"),
      });
      return;
    }
    for (const child of node.children)
      visit(child, isWord(node, "r") ? node : run);
  };
  visit(paragraph, undefined);
  return runs;
}

function plainText(node: XmlNode): string {
  if (isWord(node, "t")) return node.text;
  const text = node.children.map(plainText).join("");
  return isWord(node, "p") ? `${text}\n` : text;
}

function paragraphView(
  block: BodyBlock,
  index: Map<XmlNode, number>,
): BlockView {
  const properties = block.node.children.find((node) => isWord(node, "pPr"));
  const property = (local: string) =>
    properties?.children.find((node) => isWord(node, local));
  const alignment = valueOf(property("jc"));
  return {
    kind: "paragraph",
    id: block.id,
    expect: block.expect,
    style: valueOf(property("pStyle")) ?? "",
    list: property("numPr") !== undefined,
    ...(alignment ? { alignment } : {}),
    runs: paragraphRuns(block.node, index),
  };
}

export function documentLayout(archive: OfficePackage): DocumentLayout {
  const { document, nodes } = textNodes(archive, MAIN_PARTS.docx);
  const index = new Map(nodes.map((node, position) => [node, position]));
  return {
    blocks: bodyBlocks(document).map((block): BlockView => {
      if (isWord(block.node, "p")) return paragraphView(block, index);
      if (isWord(block.node, "tbl"))
        return {
          kind: "table",
          id: block.id,
          expect: block.expect,
          rows: block.node.children
            .filter((row) => isWord(row, "tr"))
            .map((row) =>
              row.children
                .filter((cell) => isWord(cell, "tc"))
                .map((cell) => plainText(cell).replace(/\n$/u, "")),
            ),
        };
      return {
        kind: "other",
        id: block.id,
        expect: block.expect,
        element: block.node.local,
        text: plainText(block.node).replace(/\n$/u, ""),
      };
    }),
  };
}
