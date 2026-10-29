import {
  contentLimits,
  type Format,
  type Inspection,
} from "../../contracts/content";
import { OfficePackage } from "./archive";
import { namespaces } from "./xml";

export class InspectionReport {
  readonly result: Inspection;
  private bytes = 1024;

  constructor(format: Format) {
    this.result = {
      format,
      items: [],
      facts: {},
      checks: [],
      truncated: false,
    };
  }

  item(id: string, kind: string, text: string): void {
    if (text.length > contentLimits.itemText) {
      this.result.truncated = true;
      return;
    }
    const item = { id, kind, text };
    const size = Buffer.byteLength(JSON.stringify(item), "utf8");
    if (
      this.result.items.length >= contentLimits.items ||
      this.bytes + size > contentLimits.inspectionBytes - 128 * 1024
    ) {
      this.result.truncated = true;
      return;
    }
    this.bytes += size;
    this.result.items.push(item);
  }

  check(
    code: string,
    severity: "info" | "warning" | "error",
    message: string,
    location?: string,
  ): void {
    if (this.result.checks.length >= 500) {
      this.result.truncated = true;
      return;
    }
    const check = {
      code,
      severity,
      message,
      ...(location ? { location } : {}),
    };
    const size = Buffer.byteLength(JSON.stringify(check), "utf8");
    if (this.bytes + size > contentLimits.inspectionBytes - 4096) {
      this.result.truncated = true;
      return;
    }
    this.bytes += size;
    this.result.checks.push(check);
  }

  finish(): Inspection {
    if (
      Buffer.byteLength(JSON.stringify(this.result), "utf8") >
      contentLimits.inspectionBytes
    )
      throw new Error("inspection exceeds its report byte limit");
    return this.result;
  }
}

export function officeReport(archive: OfficePackage): InspectionReport {
  const report = new InspectionReport(archive.format);
  report.result.facts.parts = archive.parts.size;
  if (archive.parts.has("docProps/core.xml")) {
    const title = archive
      .xml("docProps/core.xml")
      .nodes.find(
        (node) =>
          node.local === "title" &&
          node.uri === "http://purl.org/dc/elements/1.1/",
      )?.text;
    if (title) report.result.title = title.slice(0, 256);
  }
  report.check(
    "package_admitted",
    "info",
    "Package structure, bounded inflation, XML and relationships passed native admission.",
  );
  report.check(
    "layout_unverified",
    "warning",
    "Native package admission does not verify page layout. Render and review the exact revision.",
  );
  return report;
}

export function textParts(archive: OfficePackage): string[] {
  return [...archive.parts.keys()].filter((part) =>
    archive.format === "docx"
      ? /^word\/(?:document|header[0-9]+|footer[0-9]+|footnotes|endnotes|comments)\.xml$/u.test(
          part,
        )
      : /^ppt\/(?:slides\/slide[0-9]+|notesSlides\/notesSlide[0-9]+)\.xml$/u.test(
          part,
        ),
  );
}

export function textNodes(archive: OfficePackage, part: string) {
  const document = archive.xml(part);
  const uri = archive.format === "docx" ? namespaces.word : namespaces.drawing;
  return {
    document,
    nodes: document.nodes.filter(
      (node) => node.uri === uri && node.local === "t",
    ),
  };
}

export function inspectOfficeText(
  archive: OfficePackage,
  report: InspectionReport,
): void {
  let runs = 0;
  for (const part of textParts(archive)) {
    const { nodes } = textNodes(archive, part);
    nodes.forEach((node, index) =>
      report.item(`${part}#text:${index}`, "text", node.text),
    );
    runs += nodes.length;
  }
  report.result.facts.textRuns = runs;
}
