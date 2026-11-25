import { posix } from "node:path";
import { type OfficeFormat } from "../../contracts/content";
import {
  attribute,
  namespaces,
  parseXML,
  patchXML,
  type XmlDocument,
  type XmlNode,
  type XmlPatch,
} from "./xml";
import {
  type InflationBudget,
  decodeArchive,
  encodeArchive,
  hasControl,
  safePath,
} from "./zip";

export const MAIN_PARTS = Object.freeze({
  docx: "word/document.xml",
  xlsx: "xl/workbook.xml",
  pptx: "ppt/presentation.xml",
} as const satisfies Record<OfficeFormat, string>);

export const CONTENT_TYPES_PART = "[Content_Types].xml";

const mainTypes: Record<OfficeFormat, string> = {
  docx: "application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml",
  xlsx: "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml",
  pptx: "application/vnd.openxmlformats-officedocument.presentationml.presentation.main+xml",
};

const embeddedWorkbookType =
  "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet";

const officeDocumentRelationship = `${namespaces.officeRelationships}/officeDocument`;

const activePayload =
  /(?:vbaProject|macroEnabled|activeX|oleObject|ms-office\.vba|html|javascript|svg|externalLink|connections|queryTable)/iu;

const activeRelationship =
  /(?:oleObject|vbaProject|attachedTemplate|externalLink|connections|queryTable|control|aFChunk)$/iu;

const externalWordField =
  /(?:DDE|INCLUDETEXT|INCLUDEPICTURE|DATABASE|HYPERLINK|LINK)\b/iu;

const sheetFormulaElements = new Set([
  "f",
  "formula",
  "formula1",
  "formula2",
  "definedName",
]);

export function assertSafeFormula(formula: string): void {
  if (
    formula.length > 8192 ||
    /(?:\b(?:WEBSERVICE|HYPERLINK|IMAGE|RTD|DDE|CALL|EXEC|REGISTER(?:\.ID)?|EVALUATE|SQL\.REQUEST|CUBE[A-Z]*|STOCKHISTORY|PY)\s*\(|\[[^\]]*\][^+*/(),;]*!|\||(?:https?|file|ftp):)/iu.test(
      formula,
    )
  )
    throw new Error(
      "external-data or active spreadsheet formula is not admitted",
    );
}

export function sourceForRelationships(path: string): string {
  if (path === "_rels/.rels") return "";
  const matched = /^(.*\/)??_rels\/([^/]+)\.rels$/u.exec(path);
  if (!matched) throw new Error("invalid relationships part path");
  return `${matched[1] ?? ""}${matched[2]}`;
}

export function relationshipsPath(part: string): string {
  const separator = part.lastIndexOf("/");
  return `${part.slice(0, separator + 1)}_rels/${part.slice(separator + 1)}.rels`;
}

export function relationshipTarget(source: string, target: string): string {
  if (
    /[\\%]/u.test(target) ||
    hasControl(target) ||
    /^[A-Za-z][A-Za-z0-9+.-]*:/u.test(target)
  )
    throw new Error("unsafe package relationship target");
  const path = target.split("#", 1)[0]!;
  const result = posix.normalize(
    path.startsWith("/")
      ? path.slice(1)
      : posix.join(posix.dirname(source), path),
  );
  if (!safePath(result))
    throw new Error("package relationship escapes the archive");
  return result;
}

export interface Relation {
  id: string;
  // The final segment of the relationship type URI, such as "slide" or "chart".
  type: string;
  // Absent for external targets.
  target?: string;
  node: XmlNode;
}

export function relations(archive: OfficePackage, part: string): Relation[] {
  const path = relationshipsPath(part);
  if (!archive.parts.has(path)) return [];
  return archive.xml(path).root.children.map((node) => {
    const type = node.attributes.Type!;
    return {
      id: node.attributes.Id!,
      type: type.slice(type.lastIndexOf("/") + 1),
      ...(node.attributes.TargetMode === "External"
        ? {}
        : { target: relationshipTarget(part, node.attributes.Target!) }),
      node,
    };
  });
}

export class OfficePackage {
  readonly parts: Map<string, Uint8Array>;
  readonly format: OfficeFormat;
  private constructor(format: OfficeFormat, parts: Map<string, Uint8Array>) {
    this.format = format;
    this.parts = parts;
  }

  static open(
    format: OfficeFormat,
    bytes: Uint8Array,
    budget: InflationBudget = { bytes: 0, parts: 0 },
    depth = 0,
  ): OfficePackage {
    if (depth > 2)
      throw new Error("nested Office package depth exceeds the limit");
    const archive = new OfficePackage(format, decodeArchive(bytes, budget));
    archive.validate(budget, depth);
    return archive;
  }

  xml(path: string): XmlDocument {
    const bytes = this.parts.get(path);
    if (!bytes) throw new Error(`required Office part is missing: ${path}`);
    return parseXML(bytes);
  }

  setXML(path: string, value: string): void {
    parseXML(value);
    this.parts.set(path, Buffer.from(value, "utf8"));
  }

  // Patch offsets index the exact source parsed from the part's current bytes.
  patch(path: string, patches: XmlPatch[]): void {
    if (patches.length)
      this.setXML(path, patchXML(this.xml(path).source, patches));
  }

  save(): Uint8Array {
    return encodeArchive(this.parts);
  }

  private validate(budget: InflationBudget, depth: number): void {
    const contentType = this.declaredContentTypes();
    let mainRelationships = 0;
    for (const [path, bytes] of this.parts) {
      const type = contentType(path);
      this.admitPayload(path, type, bytes, budget, depth);
      if (!isXMLPart(path, type)) continue;
      const xml = this.xml(path);
      if (path.endsWith(".rels"))
        mainRelationships += this.admitRelationships(path, xml);
      this.assertInert(path, xml);
    }
    if (mainRelationships !== 1)
      throw new Error("Office package must have exactly one main relationship");
  }

  private declaredContentTypes(): (path: string) => string | undefined {
    const types = this.xml(CONTENT_TYPES_PART);
    if (
      types.root.uri !== namespaces.contentTypes ||
      types.root.local !== "Types"
    )
      throw new Error("invalid Office content types root");
    const defaults = new Map<string, string>();
    const overrides = new Map<string, string>();
    for (const node of types.root.children) {
      const isDefault = node.local === "Default";
      const table = isDefault
        ? defaults
        : node.local === "Override"
          ? overrides
          : undefined;
      const key = isDefault
        ? node.attributes.Extension
        : node.attributes.PartName;
      const value = node.attributes.ContentType;
      if (!table || !key || !value || table.has(key))
        throw new Error("invalid or duplicate Office content type");
      table.set(key, value);
    }
    if (
      overrides.get(`/${MAIN_PARTS[this.format]}`) !== mainTypes[this.format] ||
      !this.parts.has(MAIN_PARTS[this.format])
    )
      throw new Error(
        "native file does not match the declared macro-free Office format",
      );
    return (path) =>
      overrides.get(`/${path}`) ?? defaults.get(path.split(".").at(-1)!);
  }

  private admitPayload(
    path: string,
    contentType: string | undefined,
    bytes: Uint8Array,
    budget: InflationBudget,
    depth: number,
  ): void {
    if (path !== CONTENT_TYPES_PART && !contentType)
      throw new Error("Office part has no content type");
    if (activePayload.test(`${path} ${contentType}`))
      throw new Error("active or external-data Office payload is not admitted");
    if (!/\/embeddings\//iu.test(path)) return;
    if (!path.endsWith(".xlsx") || contentType !== embeddedWorkbookType)
      throw new Error("only macro-free embedded chart workbooks are admitted");
    OfficePackage.open("xlsx", bytes, budget, depth + 1);
  }

  // Returns how many relationships in this part name the package's main part.
  private admitRelationships(path: string, xml: XmlDocument): number {
    if (
      xml.root.local !== "Relationships" ||
      xml.root.uri !== namespaces.relationships
    )
      throw new Error("invalid Office relationship root");
    const source = sourceForRelationships(path);
    if (source && !this.parts.has(source))
      throw new Error("orphan Office relationships part");
    const ids = new Set<string>();
    let main = 0;
    for (const relation of xml.root.children)
      if (this.admitRelationship(source, relation, ids)) main++;
    return main;
  }

  // Returns whether this root relationship names the package's main part.
  private admitRelationship(
    source: string,
    relation: XmlNode,
    ids: Set<string>,
  ): boolean {
    const {
      Id: id,
      Type: type,
      Target: target,
      TargetMode: mode,
    } = relation.attributes;
    if (
      relation.local !== "Relationship" ||
      relation.uri !== namespaces.relationships ||
      !id ||
      !type ||
      !target ||
      ids.has(id)
    )
      throw new Error("invalid or duplicate Office relationship");
    ids.add(id);
    if (activeRelationship.test(type))
      throw new Error("active Office relationship is not admitted");
    if (mode === "External") {
      assertHyperlink(type, target);
      return false;
    }
    if (mode && mode !== "Internal")
      throw new Error("invalid Office relationship mode");
    const destination = relationshipTarget(source, target);
    if (!this.parts.has(destination))
      throw new Error("Office relationship targets a missing part");
    if (source || type !== officeDocumentRelationship) return false;
    if (destination !== MAIN_PARTS[this.format])
      throw new Error("Office root relationship has an unexpected main part");
    return true;
  }

  private assertInert(path: string, xml: XmlDocument): void {
    const fieldInstructions: string[] = [];
    for (const node of xml.nodes) {
      if (isFormula(node)) assertSafeFormula(node.text);
      if (
        node.uri === namespaces.word &&
        ["instrText", "fldSimple"].includes(node.local)
      )
        fieldInstructions.push(
          node.text + (attribute(node, namespaces.word, "instr") ?? ""),
        );
      for (const [key, value] of Object.entries(node.attributes))
        if (
          ["src", "href"].includes(key.split(":").at(-1)!) &&
          value &&
          !value.startsWith("#") &&
          !this.parts.has(relationshipTarget(path, value))
        )
          throw new Error("XML resource attribute leaves the Office package");
    }
    if (externalWordField.test(fieldInstructions.join("")))
      throw new Error("external Word field is not admitted");
  }
}

function isXMLPart(path: string, contentType: string | undefined): boolean {
  return (
    path.endsWith(".xml") ||
    path.endsWith(".rels") ||
    path.endsWith(".vml") ||
    contentType?.endsWith("+xml") === true
  );
}

function isFormula(node: XmlNode): boolean {
  return (
    (node.uri === namespaces.sheet && sheetFormulaElements.has(node.local)) ||
    (node.uri === namespaces.chart && node.local === "f")
  );
}

function assertHyperlink(type: string, target: string): void {
  let parsed: URL;
  try {
    parsed = new URL(target);
  } catch {
    throw new Error("invalid external hyperlink");
  }
  if (
    !type.endsWith("/hyperlink") ||
    !["http:", "https:", "mailto:"].includes(parsed.protocol) ||
    parsed.username ||
    parsed.password
  )
    throw new Error(
      "only explicit web and email hyperlinks may leave an Office package",
    );
}
