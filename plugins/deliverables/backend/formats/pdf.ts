import { inflateSync } from "node:zlib";
import {
  degrees,
  PDFArray,
  PDFCatalog,
  PDFCheckBox,
  PDFDict,
  PDFDocument,
  PDFDropdown,
  PDFField,
  PDFHexString,
  PDFName,
  PDFNumber,
  PDFObject,
  PDFOptionList,
  PDFPageTree,
  PDFRadioGroup,
  PDFRawStream,
  PDFRef,
  PDFString,
  PDFTextField,
} from "pdf-lib";
import {
  contentLimits,
  parseContent,
  pdfEditSchema,
  type Inspection,
  type PDFOperation,
} from "../../contracts/content";
import { InspectionReport } from "./inspection";
import { assertPermutation } from "./text-edits";

const forbiddenNames = new Set([
  "ObjStm",
  "XRef",
  "Encrypt",
  "JavaScript",
  "JS",
  "Launch",
  "AA",
  "OpenAction",
  "EmbeddedFiles",
  "EmbeddedFile",
  "Filespec",
  "RichMedia",
  "XFA",
  "SubmitForm",
  "ImportData",
  "GoToR",
  "GoToE",
  "Movie",
  "Sound",
  "Rendition",
  "Sig",
  "ByteRange",
]);

const isDelimiter = (character: string) =>
  "\0\t\n\f\r ()<>[]{}%/".includes(character);

function wordEnd(source: string, start: number): number {
  let index = start;
  while (index < source.length && !isDelimiter(source[index]!)) index++;
  return index;
}

function commentEnd(source: string, start: number): number {
  let index = start;
  while (
    index < source.length &&
    source[index] !== "\n" &&
    source[index] !== "\r"
  )
    index++;
  return index;
}

function literalStringEnd(source: string, start: number): number {
  let stringDepth = 1;
  let index = start + 1;
  while (index < source.length && stringDepth) {
    if (source[index] === "\\") index += 2;
    else {
      if (source[index] === "(") stringDepth++;
      else if (source[index] === ")") stringDepth--;
      index++;
    }
    if (stringDepth > contentLimits.nestingDepth)
      throw new Error("PDF literal string nesting exceeds the limit");
  }
  if (stringDepth) throw new Error("unterminated PDF literal string");
  return index;
}

function hexStringEnd(source: string, start: number): number {
  const end = source.indexOf(">", start + 1);
  if (end < 0) throw new Error("unterminated PDF hex string");
  return end + 1;
}

// Names may hide a forbidden feature behind #xx escapes, so they are decoded
// before the check.
function admittedNameEnd(source: string, start: number): number {
  const end = wordEnd(source, start + 1);
  const name = source
    .slice(start + 1, end)
    .replace(/#([0-9A-Fa-f]{2})/gu, (_, code: string) =>
      String.fromCharCode(Number.parseInt(code, 16)),
    );
  if (forbiddenNames.has(name))
    throw new Error(`PDF feature is not admitted: ${name}`);
  return end;
}

// Stream data is opaque bytes, so a keyword skips to its end marker.
function keywordEnd(source: string, start: number): number {
  const end = wordEnd(source, start + 1);
  if (source.slice(start, end) !== "stream") return end;
  const streamEnd = source.indexOf("endstream", end);
  if (streamEnd < 0) throw new Error("unterminated PDF stream");
  return streamEnd + "endstream".length;
}

function preflightPDF(bytes: Uint8Array): void {
  if (bytes.byteLength > contentLimits.fileBytes)
    throw new Error("PDF exceeds the byte limit");
  const source = Buffer.from(bytes).toString("latin1");
  if (!/^%PDF-1\.[0-7]/u.test(source) && !source.startsWith("%PDF-2.0"))
    throw new Error("file has no supported PDF header");
  let tokens = 0;
  let depth = 0;
  let index = 0;
  while (index < source.length) {
    const char = source[index]!;
    if (/\s/u.test(char) || char === "\0") {
      index++;
      continue;
    }
    if (++tokens > contentLimits.structureNodes)
      throw new Error("PDF token count exceeds the limit");
    if (char === "%") {
      index = commentEnd(source, index);
      continue;
    }
    if (char === "(") {
      index = literalStringEnd(source, index);
      continue;
    }
    if (source.startsWith("<<", index) || source.startsWith(">>", index)) {
      depth += char === "<" ? 1 : -1;
      index += 2;
    } else if (char === "[" || char === "]") {
      depth += char === "[" ? 1 : -1;
      index++;
    } else if (char === "<") index = hexStringEnd(source, index);
    else if (char === "/") index = admittedNameEnd(source, index);
    else index = keywordEnd(source, index);
    if (depth < 0 || depth > contentLimits.nestingDepth)
      throw new Error("PDF dictionary or array nesting exceeds the limit");
  }
  if (depth) throw new Error("unbalanced PDF dictionary or array");
}

const admittedStreamFilters = new Set([
  "FlateDecode",
  "DCTDecode",
  "CCITTFaxDecode",
]);

// Returns the running expanded byte total including this stream.
function admitStream(stream: PDFRawStream, expandedBefore: number): number {
  const filter = stream.dict.lookup(PDFName.of("Filter"));
  const filterName =
    filter instanceof PDFName ? filter.decodeText() : undefined;
  const data = stream.getContents();
  if (filter && !admittedStreamFilters.has(filterName ?? ""))
    throw new Error("PDF stream filter is not supported by bounded admission");
  const expanded =
    expandedBefore +
    (filterName === "FlateDecode"
      ? inflateSync(data, {
          maxOutputLength: Math.min(
            contentLimits.fileBytes,
            contentLimits.expandedBytes - expandedBefore,
          ),
        }).length
      : data.length);
  if (expanded > contentLimits.expandedBytes)
    throw new Error("PDF stream expansion exceeds the limit");
  if (stream.dict.lookup(PDFName.of("Subtype")) === PDFName.of("Image"))
    admitImageDimensions(stream.dict);
  return expanded;
}

function admitImageDimensions(dict: PDFDict): void {
  const width = dict.lookup(PDFName.of("Width"), PDFNumber).asNumber();
  const height = dict.lookup(PDFName.of("Height"), PDFNumber).asNumber();
  if (
    !Number.isInteger(width) ||
    !Number.isInteger(height) ||
    width <= 0 ||
    height <= 0 ||
    width > contentLimits.imageDimension ||
    height > contentLimits.imageDimension ||
    width * height > contentLimits.imagePixels
  )
    throw new Error("PDF image dimensions exceed the supported limits");
}

function assertSafeHyperlink(target: PDFObject | undefined): void {
  if (!(target instanceof PDFString || target instanceof PDFHexString))
    throw new Error("invalid PDF hyperlink");
  let url: URL;
  try {
    url = new URL(target.decodeText());
  } catch {
    throw new Error("invalid PDF hyperlink");
  }
  if (
    !["https:", "http:", "mailto:"].includes(url.protocol) ||
    url.username ||
    url.password
  )
    throw new Error("unsafe PDF hyperlink");
}

function validatePDFObjects(document: PDFDocument): void {
  const objects = document.context.enumerateIndirectObjects();
  if (objects.length > contentLimits.packageParts)
    throw new Error("PDF object count exceeds the limit");
  let expanded = 0;
  let visitedCount = 0;
  const visited = new Set<PDFObject>();
  const visit = (object: PDFObject | undefined, depth: number) => {
    if (!object || visited.has(object)) return;
    if (
      depth > contentLimits.nestingDepth ||
      ++visitedCount > contentLimits.structureNodes
    )
      throw new Error("PDF object graph exceeds the limit");
    visited.add(object);
    if (object instanceof PDFRef) {
      visit(document.context.lookup(object), depth + 1);
      return;
    }
    if (object instanceof PDFName && forbiddenNames.has(object.decodeText()))
      throw new Error("PDF contains an active or unsupported feature");
    if (object instanceof PDFRawStream) {
      expanded = admitStream(object, expanded);
      visit(object.dict, depth + 1);
    } else if (object instanceof PDFDict) {
      for (const [key, value] of object.entries()) {
        if (forbiddenNames.has(key.decodeText()))
          throw new Error("PDF contains an active or unsupported feature");
        if (key === PDFName.of("URI"))
          assertSafeHyperlink(document.context.lookup(value));
        visit(value, depth + 1);
      }
    } else if (object instanceof PDFArray) {
      for (let index = 0; index < object.size(); index++)
        visit(object.get(index), depth + 1);
    }
  };
  for (const [, object] of objects) visit(object, 0);
  // Page and field APIs follow parent/Kids chains recursively; validate those chains
  // before invoking them, including cycles hidden behind indirect references.
  for (const [, object] of objects) {
    const dict = object instanceof PDFDict ? object : undefined;
    if (!dict) continue;
    const parents = new Set<PDFDict>();
    let parent: PDFObject | undefined = dict;
    while (parent instanceof PDFDict) {
      if (parents.has(parent) || parents.size > contentLimits.nestingDepth)
        throw new Error("cyclic PDF parent chain");
      parents.add(parent);
      parent = parent.lookup(PDFName.of("Parent"));
    }
  }
  const ancestors = new Set<PDFDict>();
  const treeNodes = new Set<PDFDict>();
  const tree = (dict: PDFDict, depth: number) => {
    if (
      ancestors.has(dict) ||
      treeNodes.has(dict) ||
      depth > contentLimits.nestingDepth ||
      treeNodes.size > contentLimits.packageParts
    )
      throw new Error("cyclic or duplicate PDF page or form field tree");
    treeNodes.add(dict);
    ancestors.add(dict);
    const kids = dict.lookup(PDFName.of("Kids"));
    if (kids instanceof PDFArray)
      for (let index = 0; index < kids.size(); index++) {
        const child = kids.lookup(index);
        if (!(child instanceof PDFDict))
          throw new Error("invalid PDF child reference");
        tree(child, depth + 1);
      }
    ancestors.delete(dict);
  };
  // pdf-lib types the trailer's Root and its Pages only by their /Type entry, and
  // its page APIs assume both without checking.
  const catalog: unknown = document.catalog;
  if (!(catalog instanceof PDFCatalog))
    throw new Error("PDF document catalog is missing or invalid");
  const pages = catalog.lookup(PDFName.of("Pages"));
  if (!(pages instanceof PDFPageTree))
    throw new Error("PDF page tree is missing or invalid");
  tree(pages, 0);
  treeNodes.clear();
  const form = catalog.lookup(PDFName.of("AcroForm"));
  if (form instanceof PDFDict) {
    const fields = form.lookup(PDFName.of("Fields"));
    if (fields instanceof PDFArray)
      for (let index = 0; index < fields.size(); index++)
        tree(fields.lookup(index, PDFDict), 0);
  }
}

export async function loadPDF(bytes: Uint8Array): Promise<PDFDocument> {
  preflightPDF(bytes);
  const document = await PDFDocument.load(bytes, {
    ignoreEncryption: false,
    updateMetadata: false,
    throwOnInvalidObject: true,
    capNumbers: false,
  });
  validatePDFObjects(document);
  if (document.getPageCount() > 2000)
    throw new Error("PDF page count exceeds the limit");
  return document;
}

// Inspection item ids are the edit contract: a page is "page:<1-based number>"
// and a form field is "field:<fully qualified name>".
const pageItemId = (index: number) => `page:${index + 1}`;
const pageIndex = (id: string) => Number(id.slice("page:".length)) - 1;
const fieldItemId = (name: string) => `field:${name}`;
const fieldName = (id: string) => id.slice("field:".length);

function fieldDescription(field: PDFField): string {
  const readOnly = field.isReadOnly();
  if (field instanceof PDFTextField)
    return JSON.stringify({
      type: "text",
      value: field.getText() ?? "",
      readOnly,
    });
  if (field instanceof PDFCheckBox)
    return JSON.stringify({
      type: "checkbox",
      value: field.isChecked(),
      readOnly,
    });
  if (field instanceof PDFDropdown || field instanceof PDFOptionList)
    return JSON.stringify({
      type: "choice",
      value: field.getSelected(),
      options: field.getOptions(),
      readOnly,
    });
  if (field instanceof PDFRadioGroup)
    return JSON.stringify({
      type: "radio",
      value: field.getSelected() ?? "",
      options: field.getOptions(),
      readOnly,
    });
  return JSON.stringify({ type: "unsupported", readOnly });
}
function pageDescription(document: PDFDocument, index: number): string {
  const page = document.getPage(index);
  return JSON.stringify({
    width: page.getWidth(),
    height: page.getHeight(),
    rotation: page.getRotation().angle,
  });
}

export async function inspectPDF(bytes: Uint8Array): Promise<Inspection> {
  const document = await loadPDF(bytes);
  const report = new InspectionReport("pdf");
  const title = document.getTitle();
  if (title) report.result.title = title.slice(0, 256);
  for (let index = 0; index < document.getPageCount(); index++)
    report.item(pageItemId(index), "page", pageDescription(document, index));
  const fields = document.getForm().getFields();
  const names = new Set<string>();
  for (const field of fields) {
    const name = field.getName();
    if (names.has(name))
      throw new Error("PDF contains duplicate form field names");
    names.add(name);
    report.item(fieldItemId(name), "field", fieldDescription(field));
  }
  Object.assign(report.result.facts, {
    pages: document.getPageCount(),
    fields: fields.length,
    objects: document.context.enumerateIndirectObjects().length,
  });
  report.check(
    "pdf_admitted",
    "info",
    "PDF structure, passive features and bounded stream expansion passed admission.",
  );
  report.check(
    "visual_review_required",
    "warning",
    "PDF structural inspection does not assess page readability or visual layout.",
  );
  return report.finish();
}

type FieldValue = Extract<PDFOperation, { type: "fillField" }>["value"];

function fillField(field: PDFField, value: FieldValue): void {
  if (field instanceof PDFTextField && typeof value === "string")
    field.setText(value);
  else if (field instanceof PDFCheckBox && typeof value === "boolean") {
    if (value) field.check();
    else field.uncheck();
  } else if (
    (field instanceof PDFDropdown || field instanceof PDFOptionList) &&
    typeof value !== "boolean"
  ) {
    const values = typeof value === "string" ? [value] : value;
    if (values.some((choice) => !field.getOptions().includes(choice)))
      throw new Error("PDF choice is not a declared field option");
    if (values.length > 1 && !field.isMultiselect())
      throw new Error("PDF field does not allow multiple selections");
    field.select(values);
  } else if (
    field instanceof PDFRadioGroup &&
    typeof value === "string" &&
    field.getOptions().includes(value)
  )
    field.select(value);
  else throw new Error("PDF field value does not match a supported field type");
}

export async function editPDF(
  bytes: Uint8Array,
  json: string,
): Promise<Uint8Array> {
  const { operations } = parseContent(pdfEditSchema, json);
  const document = await loadPDF(bytes);
  const form = document.getForm();
  const fields = new Map(
    form.getFields().map((field) => [field.getName(), field]),
  );
  const seen = new Set<string>();
  const originalPages = document.getPages();
  let reorder: string[] | undefined;
  for (const operation of operations) {
    const key =
      operation.type === "reorderPages" ? "reorderPages" : operation.id;
    if (seen.has(key))
      throw new Error("one PDF item may be changed only once per revision");
    seen.add(key);
    if (operation.type === "reorderPages") {
      assertPermutation(
        originalPages.map((_, index) => pageItemId(index)),
        operation.ids,
      );
      reorder = operation.ids;
    } else if (operation.type === "rotatePage") {
      const index = pageIndex(operation.id);
      if (
        !originalPages[index] ||
        pageDescription(document, index) !== operation.expect
      )
        throw new Error(
          "PDF page precondition does not match the exact revision",
        );
      originalPages[index].setRotation(degrees(operation.degrees));
    } else {
      const field = fields.get(fieldName(operation.id));
      if (!field || fieldDescription(field) !== operation.expect)
        throw new Error(
          "PDF field precondition does not match the exact revision",
        );
      if (field.isReadOnly()) throw new Error("PDF form field is read-only");
      fillField(field, operation.value);
    }
  }
  if (reorder) {
    for (let index = originalPages.length - 1; index >= 0; index--)
      document.removePage(index);
    for (const id of reorder) document.addPage(originalPages[pageIndex(id)]!);
  }
  const result = await document.save({
    useObjectStreams: false,
    addDefaultPage: false,
    updateFieldAppearances: operations.some(
      (operation) => operation.type === "fillField",
    ),
  });
  await loadPDF(result);
  return result;
}
