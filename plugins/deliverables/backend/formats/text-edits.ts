import type { z } from "zod/v4";
import {
  documentEditSchema,
  presentationEditSchema,
} from "../../contracts/content";
import { OfficePackage } from "./archive";
import { textNodes, textParts } from "./inspection";
import { escapeXML, openTag, patchXML, type XmlPatch } from "./xml";

type ReplaceText = Extract<
  | z.infer<typeof documentEditSchema>["operations"][number]
  | z.infer<typeof presentationEditSchema>["operations"][number],
  { type: "replaceText" }
>;

export function replaceText(
  archive: OfficePackage,
  operations: ReplaceText[],
): void {
  const parts = new Set(textParts(archive));
  const edits = new Map<string, { source: string; patches: XmlPatch[] }>();
  const seen = new Set<string>();
  for (const operation of operations) {
    if (seen.has(operation.id))
      throw new Error(
        "one native text run may be changed only once per revision",
      );
    seen.add(operation.id);
    const match = /^(.*)#text:(0|[1-9][0-9]*)$/u.exec(operation.id);
    if (!match || !parts.has(match[1]!))
      throw new Error(
        "native text identifier does not belong to this document",
      );
    const part = match[1]!;
    const { document, nodes } = textNodes(archive, part);
    const node = nodes[Number(match[2])];
    if (!node || node.children.length || node.text !== operation.expect)
      throw new Error(
        "native text precondition does not match the exact revision",
      );
    const entry = edits.get(part) ?? { source: document.source, patches: [] };
    entry.patches.push({
      start: node.start,
      end: node.end,
      value: `${openTag(node, { ...node.attributes, "xml:space": "preserve" })}${escapeXML(operation.text)}</${node.name}>`,
    });
    edits.set(part, entry);
  }
  for (const [part, edit] of edits)
    archive.setXML(part, patchXML(edit.source, edit.patches));
}

export function assertPermutation(
  original: string[],
  requested: string[],
): void {
  if (
    original.length !== requested.length ||
    new Set(requested).size !== requested.length ||
    requested.some((id) => !original.includes(id))
  )
    throw new Error(
      "reordering requires every original item identifier exactly once",
    );
}
