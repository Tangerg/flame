import {
  AlignmentType,
  BorderStyle,
  Document,
  ExternalHyperlink,
  Footer,
  Header,
  HeadingLevel,
  ImageRun,
  Packer,
  PageBreak,
  PageNumber,
  PageOrientation,
  Paragraph,
  Table,
  TableCell,
  TableRow,
  TextRun,
  WidthType,
} from "docx";
import {
  documentContentSchema,
  documentEditSchema,
  parseContent,
  type Inspection,
} from "../../contracts/content";
import { OfficePackage } from "./archive";
import { imageBytes } from "./images";
import { inspectOfficeText, officeReport } from "./inspection";
import { replaceText } from "./text-edits";
import { escapeXML, namespaces, patchXML, tagName } from "./xml";

export async function createDocument(json: string): Promise<Uint8Array> {
  const content = parseContent(documentContentSchema, json);
  const accent = content.accent ?? "245B78";
  const font = content.font ?? "Aptos";
  const size = content.fontSize ?? 11;
  const children: Array<Paragraph | Table> = [];
  const headings = [
    HeadingLevel.HEADING_1,
    HeadingLevel.HEADING_2,
    HeadingLevel.HEADING_3,
    HeadingLevel.HEADING_4,
    HeadingLevel.HEADING_5,
    HeadingLevel.HEADING_6,
  ];
  const alignments = {
    left: AlignmentType.LEFT,
    center: AlignmentType.CENTER,
    right: AlignmentType.RIGHT,
    justify: AlignmentType.JUSTIFIED,
  };
  for (const block of content.blocks) {
    if (block.type === "paragraph") {
      children.push(
        new Paragraph({
          children: block.runs.map((run) => {
            const text = new TextRun({
              text: run.text,
              bold: run.bold,
              italics: run.italic,
              underline: run.underline ? {} : undefined,
              color: run.color,
              size: run.size ? run.size * 2 : undefined,
            });
            return run.url
              ? new ExternalHyperlink({ children: [text], link: run.url })
              : text;
          }),
          heading: block.heading ? headings[block.heading - 1] : undefined,
          alignment: block.alignment ? alignments[block.alignment] : undefined,
          bullet: block.bullet ? { level: 0 } : undefined,
          keepNext: block.keepNext,
          spacing: { after: 140, line: 276 },
        }),
      );
    } else if (block.type === "table") {
      if (block.rows.some((row) => row.length !== block.headers.length))
        throw new Error("document table rows must match the header width");
      const rows = [block.headers, ...block.rows].map(
        (row, index) =>
          new TableRow({
            tableHeader: index === 0,
            cantSplit: true,
            children: row.map(
              (cell) =>
                new TableCell({
                  shading: index === 0 ? { fill: accent } : undefined,
                  margins: { top: 100, bottom: 100, left: 120, right: 120 },
                  children: [
                    new Paragraph({
                      children: [
                        new TextRun({
                          text: cell,
                          bold: index === 0,
                          color: index === 0 ? "FFFFFF" : undefined,
                        }),
                      ],
                    }),
                  ],
                }),
            ),
          }),
      );
      children.push(
        new Table({
          rows,
          width: { size: 100, type: WidthType.PERCENTAGE },
          borders: {
            top: { style: BorderStyle.SINGLE, size: 1, color: "D3DEE5" },
            bottom: { style: BorderStyle.SINGLE, size: 1, color: "D3DEE5" },
          },
        }),
      );
    } else if (block.type === "image") {
      children.push(
        new Paragraph({
          children: [
            new ImageRun({
              type: block.image.mime === "image/png" ? "png" : "jpg",
              data: imageBytes(block.image),
              transformation: { width: block.width, height: block.height },
              altText: {
                name: block.image.alt,
                description: block.image.alt,
                title: block.caption ?? block.image.alt,
              },
            }),
          ],
          alignment: AlignmentType.CENTER,
        }),
      );
      if (block.caption)
        children.push(
          new Paragraph({
            children: [
              new TextRun({ text: block.caption, italics: true, size: 18 }),
            ],
            alignment: AlignmentType.CENTER,
          }),
        );
    } else children.push(new Paragraph({ children: [new PageBreak()] }));
  }
  if (content.references?.length) {
    if (
      new Set(content.references.map((reference) => reference.id)).size !==
      content.references.length
    )
      throw new Error("document reference identifiers must be unique");
    children.push(
      new Paragraph({ heading: HeadingLevel.HEADING_1, text: "References" }),
    );
    for (const reference of content.references) {
      const label = `[${reference.id}] ${reference.title}${reference.accessed ? ` (accessed ${reference.accessed})` : ""}`;
      children.push(
        new Paragraph({
          children: reference.url
            ? [
                new ExternalHyperlink({
                  children: [new TextRun({ text: label, style: "Hyperlink" })],
                  link: reference.url,
                }),
              ]
            : [new TextRun(label)],
        }),
      );
    }
  }
  const footer = [];
  if (content.footer) footer.push(new TextRun(content.footer));
  if (content.pageNumbers)
    footer.push(
      new TextRun({
        children: [
          content.footer ? " · " : "",
          PageNumber.CURRENT,
          " / ",
          PageNumber.TOTAL_PAGES,
        ],
      }),
    );
  const document = new Document({
    title: content.title,
    creator: content.author ?? "Flame",
    description: "Editable document created by Flame Deliverables",
    styles: {
      default: {
        document: {
          run: {
            font,
            size: size * 2,
            language: { value: content.language ?? "en-US" },
          },
        },
        heading1: {
          run: { font, size: 34, bold: true, color: accent },
          paragraph: { spacing: { before: 280, after: 160 } },
        },
        heading2: {
          run: { font, size: 28, bold: true, color: accent },
          paragraph: { spacing: { before: 220, after: 120 } },
        },
        heading3: {
          run: { font, size: 24, bold: true },
          paragraph: { spacing: { before: 180, after: 100 } },
        },
      },
    },
    sections: [
      {
        properties: {
          page: {
            size: {
              orientation: content.landscape
                ? PageOrientation.LANDSCAPE
                : PageOrientation.PORTRAIT,
            },
            margin: { top: 1080, bottom: 1080, left: 1080, right: 1080 },
          },
        },
        headers: content.header
          ? {
              default: new Header({
                children: [
                  new Paragraph({ text: content.header, style: "Header" }),
                ],
              }),
            }
          : undefined,
        footers: footer.length
          ? {
              default: new Footer({
                children: [
                  new Paragraph({
                    children: footer,
                    alignment: AlignmentType.CENTER,
                  }),
                ],
              }),
            }
          : undefined,
        children,
      },
    ],
  });
  const result = await Packer.toBuffer(document);
  OfficePackage.open("docx", result);
  return result;
}

export function inspectDocument(archive: OfficePackage): Inspection {
  const report = officeReport(archive);
  inspectOfficeText(archive, report);
  const document = archive.xml("word/document.xml");
  report.result.facts.paragraphs = document.nodes.filter(
    (node) => node.uri === namespaces.word && node.local === "p",
  ).length;
  report.result.facts.tables = document.nodes.filter(
    (node) => node.uri === namespaces.word && node.local === "tbl",
  ).length;
  report.result.facts.images = document.nodes.filter(
    (node) => node.uri === namespaces.drawing && node.local === "blip",
  ).length;
  return report.finish();
}

export function editDocument(archive: OfficePackage, json: string): Uint8Array {
  const { operations } = parseContent(documentEditSchema, json);
  replaceText(
    archive,
    operations.filter((operation) => operation.type === "replaceText"),
  );
  const appended = operations.filter(
    (operation) => operation.type === "appendParagraph",
  );
  if (appended.length) {
    const document = archive.xml("word/document.xml");
    const body = document.root.children.find(
      (node) => node.uri === namespaces.word && node.local === "body",
    );
    if (!body || body.selfClosing)
      throw new Error("document has no editable body");
    const section = body.children.find(
      (node) => node.uri === namespaces.word && node.local === "sectPr",
    );
    const p = tagName(body, "p");
    const pPr = tagName(body, "pPr");
    const pStyle = tagName(body, "pStyle");
    const r = tagName(body, "r");
    const t = tagName(body, "t");
    const val = tagName(body, "val");
    const value = appended
      .map(
        (operation) =>
          `<${p}>${operation.style ? `<${pPr}><${pStyle} ${val}="${operation.style}"/></${pPr}>` : ""}<${r}><${t} xml:space="preserve">${escapeXML(operation.text)}</${t}></${r}></${p}>`,
      )
      .join("");
    const position = section?.start ?? body.closeStart;
    archive.setXML(
      "word/document.xml",
      patchXML(document.source, [{ start: position, end: position, value }]),
    );
  }
  return archive.save();
}
