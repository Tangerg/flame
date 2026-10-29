import { posix } from "node:path";
import { crc32, deflateRawSync, inflateRawSync } from "node:zlib";
import { contentLimits, type OfficeFormat } from "../../contracts/content";
import { attribute, namespaces, parseXML, type XmlDocument } from "./xml";

interface InflationBudget {
  bytes: number;
  parts: number;
}
const utf8 = new TextDecoder("utf-8", { fatal: true });
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

function decodeArchive(
  bytes: Uint8Array,
  budget: InflationBudget,
): Map<string, Uint8Array> {
  if (bytes.byteLength > contentLimits.fileBytes)
    throw new Error("native file exceeds the byte limit");
  const buffer = Buffer.from(bytes.buffer, bytes.byteOffset, bytes.byteLength);
  let end = buffer.length - 22;
  for (; end >= Math.max(0, buffer.length - 65557); end--)
    if (
      buffer.readUInt32LE(end) === 0x06054b50 &&
      end + 22 + buffer.readUInt16LE(end + 20) === buffer.length
    )
      break;
  if (end < Math.max(0, buffer.length - 65557))
    throw new Error("ZIP end directory is missing");
  if (buffer.readUInt16LE(end + 4) || buffer.readUInt16LE(end + 6))
    throw new Error("multi-disk ZIP packages are not admitted");
  const count = buffer.readUInt16LE(end + 10);
  if (
    count !== buffer.readUInt16LE(end + 8) ||
    count === 65535 ||
    budget.parts + count > contentLimits.archiveParts
  )
    throw new Error("ZIP part count exceeds the limit or is invalid");
  const directory = buffer.readUInt32LE(end + 16);
  if (directory + buffer.readUInt32LE(end + 12) !== end)
    throw new Error("invalid ZIP central directory bounds");
  const parts = new Map<string, Uint8Array>();
  const names = new Set<string>();
  const spans: Array<{ start: number; end: number }> = [];
  let cursor = directory;
  for (let index = 0; index < count; index++) {
    if (cursor + 46 > end || buffer.readUInt32LE(cursor) !== 0x02014b50)
      throw new Error("invalid ZIP directory record");
    const flags = buffer.readUInt16LE(cursor + 8);
    const method = buffer.readUInt16LE(cursor + 10);
    const checksum = buffer.readUInt32LE(cursor + 16);
    const compressed = buffer.readUInt32LE(cursor + 20);
    const expanded = buffer.readUInt32LE(cursor + 24);
    const nameLength = buffer.readUInt16LE(cursor + 28);
    const extraLength = buffer.readUInt16LE(cursor + 30);
    const commentLength = buffer.readUInt16LE(cursor + 32);
    const offset = buffer.readUInt32LE(cursor + 42);
    const unixType = (buffer.readUInt32LE(cursor + 38) >>> 16) & 0xf000;
    if (
      (flags & ~0x080e) !== 0 ||
      ![0, 8].includes(method) ||
      buffer.readUInt16LE(cursor + 34) !== 0
    )
      throw new Error("encrypted or unsupported ZIP entry");
    if (unixType && unixType !== 0x8000 && unixType !== 0x4000)
      throw new Error("non-file ZIP entry is not admitted");
    const next = cursor + 46 + nameLength + extraLength + commentLength;
    if (next > end) throw new Error("ZIP directory entry exceeds bounds");
    const rawName = buffer.subarray(cursor + 46, cursor + 46 + nameLength);
    const name = utf8.decode(rawName);
    const isDirectory = name.endsWith("/");
    const normalized = isDirectory ? name.slice(0, -1) : name;
    if (
      !safePath(normalized) ||
      names.has(normalized.toLocaleLowerCase("en-US"))
    )
      throw new Error("unsafe or duplicate ZIP part name");
    names.add(normalized.toLocaleLowerCase("en-US"));
    checkExtra(
      buffer.subarray(
        cursor + 46 + nameLength,
        cursor + 46 + nameLength + extraLength,
      ),
    );
    if (offset + 30 > directory || buffer.readUInt32LE(offset) !== 0x04034b50)
      throw new Error("invalid ZIP local header");
    const localNameLength = buffer.readUInt16LE(offset + 26);
    const localExtraLength = buffer.readUInt16LE(offset + 28);
    const dataStart = offset + 30 + localNameLength + localExtraLength;
    let dataEnd = dataStart + compressed;
    if (
      dataEnd > directory ||
      buffer.readUInt16LE(offset + 6) !== flags ||
      buffer.readUInt16LE(offset + 8) !== method ||
      !buffer
        .subarray(offset + 30, offset + 30 + localNameLength)
        .equals(rawName)
    )
      throw new Error("ZIP local and central records disagree");
    checkExtra(buffer.subarray(offset + 30 + localNameLength, dataStart));
    if (!(flags & 8)) {
      if (
        buffer.readUInt32LE(offset + 14) !== checksum ||
        buffer.readUInt32LE(offset + 18) !== compressed ||
        buffer.readUInt32LE(offset + 22) !== expanded
      )
        throw new Error("ZIP local sizes or checksum disagree");
    } else {
      const descriptor =
        dataEnd + (buffer.readUInt32LE(dataEnd) === 0x08074b50 ? 4 : 0);
      if (
        descriptor + 12 > directory ||
        buffer.readUInt32LE(descriptor) !== checksum ||
        buffer.readUInt32LE(descriptor + 4) !== compressed ||
        buffer.readUInt32LE(descriptor + 8) !== expanded
      )
        throw new Error("invalid ZIP data descriptor");
      dataEnd = descriptor + 12;
    }
    if (
      expanded > contentLimits.fileBytes ||
      budget.bytes + expanded > contentLimits.expandedBytes
    )
      throw new Error("ZIP expansion exceeds the byte limit");
    const input = buffer.subarray(dataStart, dataStart + compressed);
    const output =
      method === 0
        ? Buffer.from(input)
        : inflateRawSync(input, {
            maxOutputLength: Math.min(
              contentLimits.fileBytes,
              contentLimits.expandedBytes - budget.bytes,
            ),
          });
    if (
      output.length !== expanded ||
      crc32(output) !== checksum ||
      (isDirectory && output.length !== 0)
    )
      throw new Error(
        "ZIP content does not match its declared size or checksum",
      );
    budget.bytes += output.length;
    budget.parts++;
    if (!isDirectory) parts.set(name, output);
    spans.push({ start: offset, end: dataEnd });
    cursor = next;
  }
  if (cursor !== end) throw new Error("ZIP directory has unaccounted entries");
  let previous = 0;
  for (const span of spans.sort((a, b) => a.start - b.start)) {
    if (span.start !== previous)
      throw new Error("ZIP entries overlap or contain unaccounted data");
    previous = span.end;
  }
  if (previous !== directory)
    throw new Error("ZIP payload does not match the central directory");
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
    const local = Buffer.alloc(30);
    local.writeUInt32LE(0x04034b50);
    local.writeUInt16LE(20, 4);
    local.writeUInt16LE(0x0800, 6);
    local.writeUInt16LE(8, 8);
    local.writeUInt16LE(33, 12);
    local.writeUInt32LE(checksum, 14);
    local.writeUInt32LE(compressed.length, 18);
    local.writeUInt32LE(bytes.byteLength, 22);
    local.writeUInt16LE(encodedName.length, 26);
    const central = Buffer.alloc(46);
    central.writeUInt32LE(0x02014b50);
    central.writeUInt16LE(20, 4);
    central.writeUInt16LE(20, 6);
    central.writeUInt16LE(0x0800, 8);
    central.writeUInt16LE(8, 10);
    central.writeUInt16LE(33, 14);
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
  const end = Buffer.alloc(22);
  end.writeUInt32LE(0x06054b50);
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

function sourceForRelationships(path: string): string {
  if (path === "_rels/.rels") return "";
  const matched = /^(.*\/)??_rels\/([^/]+)\.rels$/u.exec(path);
  if (!matched) throw new Error("invalid relationships part path");
  return `${matched[1] ?? ""}${matched[2]}`;
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

  save(): Uint8Array {
    return encodeArchive(this.parts);
  }

  private validate(budget: InflationBudget, depth: number): void {
    const types = this.xml("[Content_Types].xml");
    if (
      types.root.uri !== namespaces.contentTypes ||
      types.root.local !== "Types"
    )
      throw new Error("invalid Office content types root");
    const defaults = new Map<string, string>();
    const overrides = new Map<string, string>();
    for (const node of types.root.children) {
      const target =
        node.local === "Default"
          ? defaults
          : node.local === "Override"
            ? overrides
            : undefined;
      const key =
        node.local === "Default"
          ? node.attributes.Extension
          : node.attributes.PartName;
      const value = node.attributes.ContentType;
      if (!target || !key || !value || target.has(key))
        throw new Error("invalid or duplicate Office content type");
      target.set(key, value);
    }
    if (
      overrides.get(`/${mainParts[this.format]}`) !== mainTypes[this.format] ||
      !this.parts.has(mainParts[this.format])
    )
      throw new Error(
        "native file does not match the declared macro-free Office format",
      );
    let mainRelationship = 0;
    for (const [path, bytes] of this.parts) {
      const contentType =
        overrides.get(`/${path}`) ?? defaults.get(path.split(".").at(-1)!);
      if (path !== "[Content_Types].xml" && !contentType)
        throw new Error("Office part has no content type");
      if (
        /(?:vbaProject|macroEnabled|activeX|oleObject|ms-office\.vba|html|javascript|svg|externalLink|connections|queryTable)/iu.test(
          `${path} ${contentType}`,
        )
      )
        throw new Error(
          "active or external-data Office payload is not admitted",
        );
      if (/\/embeddings\//iu.test(path)) {
        if (
          !path.endsWith(".xlsx") ||
          contentType !==
            "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"
        )
          throw new Error(
            "only macro-free embedded chart workbooks are admitted",
          );
        OfficePackage.open("xlsx", bytes, budget, depth + 1);
      }
      if (
        !path.endsWith(".xml") &&
        !path.endsWith(".rels") &&
        !path.endsWith(".vml") &&
        !contentType?.endsWith("+xml")
      )
        continue;
      const xml = this.xml(path);
      if (path.endsWith(".rels")) {
        if (
          xml.root.local !== "Relationships" ||
          xml.root.uri !== namespaces.relationships
        )
          throw new Error("invalid Office relationship root");
        const source = sourceForRelationships(path);
        if (source && !this.parts.has(source))
          throw new Error("orphan Office relationships part");
        const ids = new Set<string>();
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
          if (
            /(?:oleObject|vbaProject|attachedTemplate|externalLink|connections|queryTable|control|aFChunk)$/iu.test(
              type,
            )
          )
            throw new Error("active Office relationship is not admitted");
          if (mode === "External") {
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
            continue;
          }
          if (mode && mode !== "Internal")
            throw new Error("invalid Office relationship mode");
          const destination = relationshipTarget(source, target);
          if (!this.parts.has(destination))
            throw new Error("Office relationship targets a missing part");
          if (
            !source &&
            type === `${namespaces.officeRelationships}/officeDocument`
          ) {
            if (destination !== mainParts[this.format])
              throw new Error(
                "Office root relationship has an unexpected main part",
              );
            mainRelationship++;
          }
        }
      }
      const fieldInstructions: string[] = [];
      for (const node of xml.nodes) {
        if (
          (node.uri === namespaces.sheet &&
            ["f", "formula", "formula1", "formula2", "definedName"].includes(
              node.local,
            )) ||
          (node.uri === namespaces.chart && node.local === "f")
        )
          assertSafeFormula(node.text);
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
            !value.startsWith("#")
          ) {
            const destination = relationshipTarget(path, value);
            if (!this.parts.has(destination))
              throw new Error(
                "XML resource attribute leaves the Office package",
              );
          }
      }
      if (
        /(?:DDE|INCLUDETEXT|INCLUDEPICTURE|DATABASE|HYPERLINK|LINK)\b/iu.test(
          fieldInstructions.join(""),
        )
      )
        throw new Error("external Word field is not admitted");
    }
    if (mainRelationship !== 1)
      throw new Error("Office package must have exactly one main relationship");
  }
}
