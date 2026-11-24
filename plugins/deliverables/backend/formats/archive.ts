import { posix } from "node:path";
import { crc32, deflateRawSync, inflateRawSync } from "node:zlib";
import { contentLimits, type OfficeFormat } from "../../contracts/content";
import {
  attribute,
  namespaces,
  parseXML,
  patchXML,
  type XmlDocument,
  type XmlNode,
  type XmlPatch,
} from "./xml";

interface InflationBudget {
  bytes: number;
  parts: number;
}
const utf8 = new TextDecoder("utf-8", { fatal: true });
const signatures = {
  local: 0x04034b50,
  central: 0x02014b50,
  descriptor: 0x08074b50,
  end: 0x06054b50,
} as const;
const methods = { stored: 0, deflated: 8 } as const;
const localHeaderBytes = 30;
const centralHeaderBytes = 46;
const endRecordBytes = 22;
// The end record may be followed by an archive comment of up to 65535 bytes.
const endRecordSearchBytes = endRecordBytes + 0xffff;
const dataDescriptorFlag = 0x0008;
const utf8NameFlag = 0x0800;
const admittedFlags = 0x0006 | dataDescriptorFlag | utf8NameFlag;
const unixRegularFile = 0x8000;
const unixDirectory = 0x4000;
const zipVersion = 20;
// A fixed DOS date (1980-01-01) keeps saved packages byte-for-byte reproducible.
const fixedDosDate = 33;
const mainParts: Record<OfficeFormat, string> = {
  docx: "word/document.xml",
  xlsx: "xl/workbook.xml",
  pptx: "ppt/presentation.xml",
};
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

function safePath(name: string): boolean {
  return (
    name.length > 0 &&
    name.length <= 1024 &&
    name.normalize("NFC") === name &&
    !/[\\%:]/u.test(name) &&
    !hasControl(name) &&
    !name.startsWith("/") &&
    !name
      .split("/")
      .some((part) => part === "." || part === ".." || part === "")
  );
}

function hasControl(value: string): boolean {
  return [...value].some(
    (character) =>
      character.charCodeAt(0) < 32 || character.charCodeAt(0) === 127,
  );
}

function checkExtra(buffer: Buffer): void {
  let position = 0;
  while (position < buffer.length) {
    if (position + 4 > buffer.length)
      throw new Error("invalid ZIP extra field");
    const id = buffer.readUInt16LE(position);
    const length = buffer.readUInt16LE(position + 2);
    if (id === 1) throw new Error("ZIP64 packages are not admitted");
    position += 4 + length;
  }
  if (position !== buffer.length)
    throw new Error("invalid ZIP extra field length");
}

interface CentralDirectory {
  start: number;
  end: number;
  count: number;
}

interface CentralRecord {
  name: string;
  rawName: Buffer;
  isDirectory: boolean;
  flags: number;
  method: number;
  checksum: number;
  compressed: number;
  expanded: number;
  offset: number;
  next: number;
}

function locateEndRecord(buffer: Buffer): number {
  const lowest = Math.max(0, buffer.length - endRecordSearchBytes);
  for (let end = buffer.length - endRecordBytes; end >= lowest; end--)
    if (
      buffer.readUInt32LE(end) === signatures.end &&
      end + endRecordBytes + buffer.readUInt16LE(end + 20) === buffer.length
    )
      return end;
  throw new Error("ZIP end directory is missing");
}

function readCentralDirectory(
  buffer: Buffer,
  budget: InflationBudget,
): CentralDirectory {
  const end = locateEndRecord(buffer);
  if (buffer.readUInt16LE(end + 4) || buffer.readUInt16LE(end + 6))
    throw new Error("multi-disk ZIP packages are not admitted");
  const count = buffer.readUInt16LE(end + 10);
  if (
    count !== buffer.readUInt16LE(end + 8) ||
    count === 0xffff ||
    budget.parts + count > contentLimits.archiveParts
  )
    throw new Error("ZIP part count exceeds the limit or is invalid");
  const start = buffer.readUInt32LE(end + 16);
  if (start + buffer.readUInt32LE(end + 12) !== end)
    throw new Error("invalid ZIP central directory bounds");
  return { start, end, count };
}

function readCentralRecord(
  buffer: Buffer,
  cursor: number,
  directory: CentralDirectory,
  names: Set<string>,
): CentralRecord {
  if (
    cursor + centralHeaderBytes > directory.end ||
    buffer.readUInt32LE(cursor) !== signatures.central
  )
    throw new Error("invalid ZIP directory record");
  const flags = buffer.readUInt16LE(cursor + 8);
  const method = buffer.readUInt16LE(cursor + 10);
  const unixType = (buffer.readUInt32LE(cursor + 38) >>> 16) & 0xf000;
  if (
    (flags & ~admittedFlags) !== 0 ||
    (method !== methods.stored && method !== methods.deflated) ||
    buffer.readUInt16LE(cursor + 34) !== 0
  )
    throw new Error("encrypted or unsupported ZIP entry");
  if (unixType && unixType !== unixRegularFile && unixType !== unixDirectory)
    throw new Error("non-file ZIP entry is not admitted");
  const nameStart = cursor + centralHeaderBytes;
  const extraStart = nameStart + buffer.readUInt16LE(cursor + 28);
  const extraEnd = extraStart + buffer.readUInt16LE(cursor + 30);
  const next = extraEnd + buffer.readUInt16LE(cursor + 32);
  if (next > directory.end)
    throw new Error("ZIP directory entry exceeds bounds");
  const rawName = buffer.subarray(nameStart, extraStart);
  const name = utf8.decode(rawName);
  const isDirectory = name.endsWith("/");
  const normalized = isDirectory ? name.slice(0, -1) : name;
  // Case-insensitive file systems would merge parts that differ only in case.
  const key = normalized.toLocaleLowerCase("en-US");
  if (!safePath(normalized) || names.has(key))
    throw new Error("unsafe or duplicate ZIP part name");
  names.add(key);
  checkExtra(buffer.subarray(extraStart, extraEnd));
  return {
    name,
    rawName,
    isDirectory,
    flags,
    method,
    checksum: buffer.readUInt32LE(cursor + 16),
    compressed: buffer.readUInt32LE(cursor + 20),
    expanded: buffer.readUInt32LE(cursor + 24),
    offset: buffer.readUInt32LE(cursor + 42),
    next,
  };
}

// Returns where the entry's local record ends, after any data descriptor.
function localRecordEnd(
  buffer: Buffer,
  record: CentralRecord,
  dataEnd: number,
  limit: number,
): number {
  if (!(record.flags & dataDescriptorFlag)) {
    if (
      buffer.readUInt32LE(record.offset + 14) !== record.checksum ||
      buffer.readUInt32LE(record.offset + 18) !== record.compressed ||
      buffer.readUInt32LE(record.offset + 22) !== record.expanded
    )
      throw new Error("ZIP local sizes or checksum disagree");
    return dataEnd;
  }
  const descriptor =
    dataEnd + (buffer.readUInt32LE(dataEnd) === signatures.descriptor ? 4 : 0);
  if (
    descriptor + 12 > limit ||
    buffer.readUInt32LE(descriptor) !== record.checksum ||
    buffer.readUInt32LE(descriptor + 4) !== record.compressed ||
    buffer.readUInt32LE(descriptor + 8) !== record.expanded
  )
    throw new Error("invalid ZIP data descriptor");
  return descriptor + 12;
}

function inflateEntry(
  input: Buffer,
  record: CentralRecord,
  budget: InflationBudget,
): Buffer {
  if (
    record.expanded > contentLimits.fileBytes ||
    budget.bytes + record.expanded > contentLimits.expandedBytes
  )
    throw new Error("ZIP expansion exceeds the byte limit");
  const output =
    record.method === methods.stored
      ? Buffer.from(input)
      : inflateRawSync(input, {
          maxOutputLength: Math.min(
            contentLimits.fileBytes,
            contentLimits.expandedBytes - budget.bytes,
          ),
        });
  if (
    output.length !== record.expanded ||
    crc32(output) !== record.checksum ||
    (record.isDirectory && output.length !== 0)
  )
    throw new Error("ZIP content does not match its declared size or checksum");
  return output;
}

function readEntry(
  buffer: Buffer,
  record: CentralRecord,
  limit: number,
  budget: InflationBudget,
): { output: Buffer; end: number } {
  const { offset } = record;
  if (
    offset + localHeaderBytes > limit ||
    buffer.readUInt32LE(offset) !== signatures.local
  )
    throw new Error("invalid ZIP local header");
  const nameStart = offset + localHeaderBytes;
  const extraStart = nameStart + buffer.readUInt16LE(offset + 26);
  const dataStart = extraStart + buffer.readUInt16LE(offset + 28);
  const dataEnd = dataStart + record.compressed;
  if (
    dataEnd > limit ||
    buffer.readUInt16LE(offset + 6) !== record.flags ||
    buffer.readUInt16LE(offset + 8) !== record.method ||
    !buffer.subarray(nameStart, extraStart).equals(record.rawName)
  )
    throw new Error("ZIP local and central records disagree");
  checkExtra(buffer.subarray(extraStart, dataStart));
  const end = localRecordEnd(buffer, record, dataEnd, limit);
  const output = inflateEntry(
    buffer.subarray(dataStart, dataEnd),
    record,
    budget,
  );
  return { output, end };
}

// Every payload byte must belong to exactly one entry, so no hidden data can
// travel between, before or after the declared parts.
function assertContiguous(
  spans: Array<{ start: number; end: number }>,
  directoryStart: number,
): void {
  let previous = 0;
  for (const span of spans.sort((a, b) => a.start - b.start)) {
    if (span.start !== previous)
      throw new Error("ZIP entries overlap or contain unaccounted data");
    previous = span.end;
  }
  if (previous !== directoryStart)
    throw new Error("ZIP payload does not match the central directory");
}

function decodeArchive(
  bytes: Uint8Array,
  budget: InflationBudget,
): Map<string, Uint8Array> {
  if (bytes.byteLength > contentLimits.fileBytes)
    throw new Error("native file exceeds the byte limit");
  const buffer = Buffer.from(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  const directory = readCentralDirectory(buffer, budget);
  const parts = new Map<string, Uint8Array>();
  const names = new Set<string>();
  const spans: Array<{ start: number; end: number }> = [];
  let cursor = directory.start;
  for (let index = 0; index < directory.count; index++) {
    const record = readCentralRecord(buffer, cursor, directory, names);
    const entry = readEntry(buffer, record, directory.start, budget);
    budget.bytes += entry.output.length;
    budget.parts++;
    if (!record.isDirectory) parts.set(record.name, entry.output);
    spans.push({ start: record.offset, end: entry.end });
    cursor = record.next;
  }
  if (cursor !== directory.end)
    throw new Error("ZIP directory has unaccounted entries");
  assertContiguous(spans, directory.start);
  return parts;
}

export function encodeArchive(
  parts: ReadonlyMap<string, Uint8Array>,
  maximumFileBytes: number = contentLimits.fileBytes,
): Uint8Array {
  if (parts.size > contentLimits.archiveParts)
    throw new Error("ZIP part count exceeds the limit");
  const payload: Buffer[] = [];
  const directory: Buffer[] = [];
  let offset = 0;
  let expanded = 0;
  const names = new Set<string>();
  for (const [name, bytes] of parts) {
    if (!safePath(name) || names.has(name.toLocaleLowerCase("en-US")))
      throw new Error("unsafe or duplicate ZIP part name");
    names.add(name.toLocaleLowerCase("en-US"));
    expanded += bytes.byteLength;
    if (
      expanded > contentLimits.expandedBytes ||
      bytes.byteLength > contentLimits.fileBytes
    )
      throw new Error("ZIP expansion exceeds the byte limit");
    const encodedName = Buffer.from(name, "utf8");
    const compressed = deflateRawSync(bytes, { level: 6 });
    const checksum = crc32(bytes);
    const local = Buffer.alloc(localHeaderBytes);
    local.writeUInt32LE(signatures.local);
    local.writeUInt16LE(zipVersion, 4);
    local.writeUInt16LE(utf8NameFlag, 6);
    local.writeUInt16LE(methods.deflated, 8);
    local.writeUInt16LE(fixedDosDate, 12);
    local.writeUInt32LE(checksum, 14);
    local.writeUInt32LE(compressed.length, 18);
    local.writeUInt32LE(bytes.byteLength, 22);
    local.writeUInt16LE(encodedName.length, 26);
    const central = Buffer.alloc(centralHeaderBytes);
    central.writeUInt32LE(signatures.central);
    central.writeUInt16LE(zipVersion, 4);
    central.writeUInt16LE(zipVersion, 6);
    central.writeUInt16LE(utf8NameFlag, 8);
    central.writeUInt16LE(methods.deflated, 10);
    central.writeUInt16LE(fixedDosDate, 14);
    central.writeUInt32LE(checksum, 16);
    central.writeUInt32LE(compressed.length, 20);
    central.writeUInt32LE(bytes.byteLength, 24);
    central.writeUInt16LE(encodedName.length, 28);
    central.writeUInt32LE(offset, 42);
    payload.push(local, encodedName, compressed);
    directory.push(central, encodedName);
    offset += local.length + encodedName.length + compressed.length;
  }
  const directoryBytes = Buffer.concat(directory);
  const end = Buffer.alloc(endRecordBytes);
  end.writeUInt32LE(signatures.end);
  end.writeUInt16LE(parts.size, 8);
  end.writeUInt16LE(parts.size, 10);
  end.writeUInt32LE(directoryBytes.length, 12);
  end.writeUInt32LE(offset, 16);
  const result = Buffer.concat([...payload, directoryBytes, end]);
  if (result.length > maximumFileBytes)
    throw new Error("archive exceeds the byte limit");
  return result;
}

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
    const types = this.xml("[Content_Types].xml");
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
      overrides.get(`/${mainParts[this.format]}`) !== mainTypes[this.format] ||
      !this.parts.has(mainParts[this.format])
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
    if (path !== "[Content_Types].xml" && !contentType)
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
    for (const relation of xml.root.children) {
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
        continue;
      }
      if (mode && mode !== "Internal")
        throw new Error("invalid Office relationship mode");
      const destination = relationshipTarget(source, target);
      if (!this.parts.has(destination))
        throw new Error("Office relationship targets a missing part");
      if (source || type !== officeDocumentRelationship) continue;
      if (destination !== mainParts[this.format])
        throw new Error("Office root relationship has an unexpected main part");
      main++;
    }
    return main;
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
