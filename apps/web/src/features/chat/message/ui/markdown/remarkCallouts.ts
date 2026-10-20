import { type Blockquote, type Root } from "mdast";
import { visit } from "unist-util-visit";
import { readCalloutMarker } from "@flame/ui";
function authoredMarker(quote: Blockquote, source: string) {
  const lead = quote.children[0];
  if (lead?.type !== "paragraph") return null;
  const start = lead.position?.start.offset;
  const end = lead.position?.end.offset;
  if (start === undefined || end === undefined) return null;
  return readCalloutMarker(source.slice(start, end));
}

function claimCallout(quote: Blockquote, source: string): void {
  const authored = authoredMarker(quote, source);
  if (!authored) return;
  const lead = quote.children[0];
  if (lead?.type !== "paragraph") return;
  const text = lead.children[0];
  if (text?.type !== "text") return;
  const marker = readCalloutMarker(text.value);
  if (marker?.kind !== authored.kind) return;
  if (marker.rest) text.value = marker.rest;
  else lead.children.splice(0, lead.children[1]?.type === "break" ? 2 : 1);
  if (lead.children.length === 0) quote.children.shift();
  quote.data = { ...quote.data, hProperties: { dataCallout: marker.kind } };
}

export function remarkCallouts() {
  return (tree: Root, file: { value: unknown }): void => {
    const source = String(file.value);
    visit(tree, "blockquote", (quote) => claimCallout(quote, source));
  };
}
