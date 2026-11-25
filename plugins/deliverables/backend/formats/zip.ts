import { crc32, deflateRawSync, inflateRawSync } from "node:zlib";
import { contentLimits } from "../../contracts/content";

export interface InflationBudget {
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

export function safePath(name: string): boolean {
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

export function hasControl(value: string): boolean {
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
    budget.parts + count > contentLimits.packageParts
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

export function decodeArchive(
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
  if (parts.size > contentLimits.packageParts)
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
