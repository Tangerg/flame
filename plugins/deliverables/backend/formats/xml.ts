import { SaxesParser } from "saxes";
import { contentLimits } from "../../contracts/content";

export const namespaces = {
  word: "http://schemas.openxmlformats.org/wordprocessingml/2006/main",
  sheet: "http://schemas.openxmlformats.org/spreadsheetml/2006/main",
  presentation: "http://schemas.openxmlformats.org/presentationml/2006/main",
  drawing: "http://schemas.openxmlformats.org/drawingml/2006/main",
  chart: "http://schemas.openxmlformats.org/drawingml/2006/chart",
  relationships: "http://schemas.openxmlformats.org/package/2006/relationships",
  officeRelationships:
    "http://schemas.openxmlformats.org/officeDocument/2006/relationships",
  contentTypes: "http://schemas.openxmlformats.org/package/2006/content-types",
} as const;

export interface XmlNode {
  name: string;
  local: string;
  uri: string;
  attributes: Record<string, string>;
  qualifiedAttributes: Record<string, string>;
  start: number;
  openEnd: number;
  closeStart: number;
  end: number;
  selfClosing: boolean;
  text: string;
  children: XmlNode[];
}
export interface XmlDocument {
  source: string;
  root: XmlNode;
  nodes: XmlNode[];
}
export interface XmlPatch {
  start: number;
  end: number;
  value: string;
}

export function parseXML(bytes: Uint8Array | string): XmlDocument {
  if (typeof bytes !== "string" && bytes.byteLength > contentLimits.xmlBytes)
    throw new Error("XML part exceeds the byte limit");
  const source =
    typeof bytes === "string"
      ? bytes
      : new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  if (Buffer.byteLength(source, "utf8") > contentLimits.xmlBytes)
    throw new Error("XML part exceeds the byte limit");
  const parser = new SaxesParser({ xmlns: true });
  const stack: XmlNode[] = [];
  const nodes: XmlNode[] = [];
  let root: XmlNode | undefined;
  parser.on("doctype", () => {
    throw new Error("XML document types are not admitted");
  });
  parser.on("processinginstruction", () => {
    throw new Error("XML processing instructions are not admitted");
  });
  parser.on("xmldecl", (declaration) => {
    if (declaration.encoding && !/^utf-?8$/iu.test(declaration.encoding))
      throw new Error("only UTF-8 XML parts are admitted");
  });
  parser.on("error", (error) => {
    throw new Error("invalid XML part", { cause: error });
  });
  parser.on("opentag", (tag) => {
    if (nodes.length >= contentLimits.xmlNodes || stack.length >= 128)
      throw new Error("XML structure exceeds the node or depth limit");
    const node: XmlNode = {
      name: tag.name,
      local: tag.local,
      uri: tag.uri,
      attributes: Object.fromEntries(
        Object.entries(tag.attributes).map(([name, value]) => [
          name,
          value.value,
        ]),
      ),
      qualifiedAttributes: Object.fromEntries(
        Object.values(tag.attributes).map((value) => [
          `${value.uri}|${value.local}`,
          value.value,
        ]),
      ),
      start: source.lastIndexOf("<", parser.position - 1),
      openEnd: parser.position,
      closeStart: parser.position,
      end: parser.position,
      selfClosing: tag.isSelfClosing,
      text: "",
      children: [],
    };
    if (stack.length) stack.at(-1)!.children.push(node);
    else root = node;
    nodes.push(node);
    stack.push(node);
  });
  const appendText = (value: string) => {
    const current = stack.at(-1);
    if (current) current.text += value;
  };
  parser.on("text", appendText);
  parser.on("cdata", appendText);
  parser.on("closetag", () => {
    const node = stack.pop()!;
    node.end = parser.position;
    node.closeStart = node.selfClosing
      ? node.openEnd
      : source.lastIndexOf("<", parser.position - 1);
  });
  parser.write(source).close();
  if (!root) throw new Error("XML part has no root element");
  return { source, root, nodes };
}

export function escapeXML(value: string): string {
  if (
    !value.isWellFormed() ||
    [...value].some((character) => {
      const code = character.codePointAt(0)!;
      return (
        code === 0xfffe ||
        code === 0xffff ||
        (code < 32 && ![9, 10, 13].includes(code))
      );
    })
  )
    throw new Error("text contains characters forbidden in XML");
  return value
    .replace(/&/gu, "&amp;")
    .replace(/</gu, "&lt;")
    .replace(/>/gu, "&gt;")
    .replace(/"/gu, "&quot;")
    .replace(/'/gu, "&apos;");
}

export function patchXML(source: string, patches: XmlPatch[]): string {
  let previous = source.length;
  let result = source;
  for (const patch of [...patches].sort(
    (a, b) => b.start - a.start || b.end - a.end,
  )) {
    if (patch.start < 0 || patch.end < patch.start || patch.end > previous)
      throw new Error("native edits overlap or target an invalid XML span");
    result =
      result.slice(0, patch.start) + patch.value + result.slice(patch.end);
    previous = patch.start;
  }
  return result;
}

export function tagName(node: XmlNode, local: string): string {
  const separator = node.name.indexOf(":");
  return separator < 0 ? local : `${node.name.slice(0, separator)}:${local}`;
}

export function childrenNamed(
  node: XmlNode,
  uri: string,
  local: string,
): XmlNode[] {
  return node.children.filter(
    (child) => child.uri === uri && child.local === local,
  );
}

export function openTag(node: XmlNode, attributes = node.attributes): string {
  return `<${node.name}${Object.entries(attributes)
    .map(([name, value]) => ` ${name}="${escapeXML(value)}"`)
    .join("")}>`;
}

export function nodeText(node: XmlNode): string {
  return node.text + node.children.map(nodeText).join("");
}

export function attribute(
  node: XmlNode,
  uri: string,
  local: string,
): string | undefined {
  return node.qualifiedAttributes[`${uri}|${local}`];
}
