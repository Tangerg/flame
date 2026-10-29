import { Children, cloneElement, isValidElement, type ReactNode } from "react";
import type { Blockquote, Root } from "mdast";
import { visit } from "unist-util-visit";
import { readCalloutMarker } from "./callout";
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

export function visibleText(children: ReactNode): string {
  return Children.toArray(children)
    .map((child) => {
      if (typeof child === "string" || typeof child === "number")
        return String(child);
      return isValidElement<{ children?: ReactNode }>(child)
        ? visibleText(child.props.children)
        : "";
    })
    .join("");
}

const WHITESPACE_ONLY = /^\s*$/;

type MarkdownElementProps = {
  children?: ReactNode;
  node?: { tagName?: string };
  checked?: boolean;
  type?: string;
};

const BLOCK_TAGS = new Set([
  "p",
  "ul",
  "ol",
  "blockquote",
  "pre",
  "table",
  "h1",
  "h2",
  "h3",
  "h4",
  "h5",
  "h6",
  "hr",
]);

export function taskListContent(
  children: ReactNode,
): { checked: boolean; label: string; children: ReactNode } | null {
  const items = Children.toArray(children).filter(
    (child) => !(typeof child === "string" && WHITESPACE_ONLY.test(child)),
  );
  const lead = items[0];
  const paragraph =
    isValidElement<MarkdownElementProps>(lead) &&
    lead.props.node?.tagName === "p"
      ? lead
      : null;
  const [checkbox, ...content] = paragraph
    ? Children.toArray(paragraph.props.children)
    : items;
  if (
    !isValidElement<MarkdownElementProps>(checkbox) ||
    (checkbox.type !== "input" && checkbox.props.node?.tagName !== "input") ||
    checkbox.props.type !== "checkbox"
  ) {
    return null;
  }

  const blockIndex = content.findIndex(
    (child) =>
      isValidElement<MarkdownElementProps>(child) &&
      BLOCK_TAGS.has(child.props.node?.tagName ?? String(child.type)),
  );
  return {
    checked: checkbox.props.checked === true,
    label: visibleText(
      paragraph || blockIndex < 0 ? content : content.slice(0, blockIndex),
    ).trim(),
    children: paragraph
      ? [cloneElement(paragraph, { children: content }), ...items.slice(1)]
      : content,
  };
}
