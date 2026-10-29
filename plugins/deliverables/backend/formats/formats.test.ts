import { PDFDocument, StandardFonts, degrees } from "pdf-lib";
import { deflateRawSync } from "node:zlib";
import { describe, expect, it } from "vitest";
import {
  contentLimits,
  formatCatalogSchema,
  inspectionSchema,
} from "../../contracts/content";
import { OfficePackage, encodeArchive } from "./archive";
import { catalog, createFile, editFile, exportCSV, inspectFile } from "./index";
import { parseXML } from "./xml";

const specification = (format: "docx" | "xlsx" | "pptx") =>
  catalog().formats.find((entry) => entry.format === format)!.exampleJSON!;

describe("native deliverable formats", () => {
  it.each(["docx", "xlsx", "pptx"] as const)(
    "creates and inspects the complete %s catalog example",
    async (format) => {
      const bytes = await createFile(format, specification(format));
      const inspection = await inspectFile(format, bytes);
      expect(bytes.length).toBeGreaterThan(1000);
      expect(inspection.format).toBe(format);
      expect(inspection.title).toBeTruthy();
      expect(inspection.items.length).toBeGreaterThan(0);
      expect(inspectionSchema.safeParse(inspection).success).toBe(true);
    },
  );

  it("publishes schemas and examples from the format contract owner", () => {
    const values = catalog();
    expect(formatCatalogSchema.parse(values)).toEqual(values);
    for (const format of values.formats) {
      expect(JSON.parse(format.editSchemaJSON).type).toBe("object");
      expect(format.limitations.length).toBeGreaterThan(0);
    }
  });

  it("edits an exact DOCX run while retaining unrelated native entries", async () => {
    const bytes = await createFile("docx", specification("docx"));
    const before = OfficePackage.open("docx", bytes);
    const inspection = await inspectFile("docx", bytes);
    const item = inspection.items.find(
      (item) => item.kind === "text" && item.id.startsWith("word/document.xml"),
    )!;
    const result = await editFile(
      "docx",
      bytes,
      JSON.stringify({
        operations: [
          {
            type: "replaceText",
            id: item.id,
            expect: item.text,
            text: "Revised & reviewed <heading>",
          },
          { type: "appendParagraph", style: "Heading2", text: "Decision" },
        ],
      }),
    );
    const after = OfficePackage.open("docx", result);
    for (const [name, value] of before.parts)
      if (name !== "word/document.xml")
        expect(after.parts.get(name)).toEqual(value);
    expect(
      (await inspectFile("docx", result)).items.some(
        (item) => item.text === "Revised & reviewed <heading>",
      ),
    ).toBe(true);
    await expect(
      editFile(
        "docx",
        result,
        JSON.stringify({
          operations: [
            {
              type: "replaceText",
              id: item.id,
              expect: item.text,
              text: "stale",
            },
          ],
        }),
      ),
    ).rejects.toThrow("precondition");
  });

  it("creates a native XLSX chart whose series reference the authored cells", async () => {
    const bytes = await createFile("xlsx", specification("xlsx"));
    const archive = OfficePackage.open("xlsx", bytes);
    const chart = archive.xml("xl/charts/chart1.xml");
    expect(chart.source).toContain("&apos;Revenue&apos;!$B$2:$B$3");
    expect(chart.source).toContain("<c:barChart>");
    expect(chart.source).toContain("<c:v>112</c:v>");
    expect(archive.xml("xl/drawings/drawing1.xml").source).toContain(
      "<c:chart",
    );
    expect((await inspectFile("xlsx", bytes)).facts.charts).toBe(1);
  });

  it("preserves styles and chart relationships while invalidating every formula cache", async () => {
    const bytes = await createFile("xlsx", specification("xlsx"));
    const original = OfficePackage.open("xlsx", bytes);
    const worksheet = original.xml("xl/worksheets/sheet1.xml");
    original.setXML(
      "xl/worksheets/sheet1.xml",
      worksheet.source.replace(
        "<f>SUM(B2:B3)</f>",
        "<f>SUM(B2:B3)</f><v>212</v>",
      ),
    );
    const imported = original.save();
    const inspection = await inspectFile("xlsx", imported);
    const item = inspection.items.find((item) => item.id.endsWith("!B2"))!;
    const result = await editFile(
      "xlsx",
      imported,
      JSON.stringify({
        operations: [
          {
            type: "setCell",
            id: item.id,
            expect: item.text,
            value: { type: "number", value: 120 },
          },
          {
            type: "setCell",
            id: "xl/worksheets/sheet1.xml!C7",
            expect: "",
            value: { type: "string", value: "New & exact" },
          },
        ],
      }),
    );
    const changed = OfficePackage.open("xlsx", result);
    expect(changed.parts.get("xl/styles.xml")).toEqual(
      original.parts.get("xl/styles.xml"),
    );
    expect(changed.parts.get("xl/drawings/drawing1.xml")).toEqual(
      original.parts.get("xl/drawings/drawing1.xml"),
    );
    expect(changed.parts.get("xl/drawings/_rels/drawing1.xml.rels")).toEqual(
      original.parts.get("xl/drawings/_rels/drawing1.xml.rels"),
    );
    expect(changed.xml("xl/worksheets/sheet1.xml").source).not.toContain(
      "<v>212</v>",
    );
    expect(changed.xml("xl/charts/chart1.xml").source).not.toContain(
      "numCache",
    );
    const after = await inspectFile("xlsx", result);
    expect(after.facts.missingFormulaCaches).toBe(1);
    expect(after.items.find((item) => item.id.endsWith("!C7"))?.text).toContain(
      "New & exact",
    );
  });

  it("does not let duplicate cell edits create a second value owner", async () => {
    const bytes = await createFile("xlsx", specification("xlsx"));
    const item = (await inspectFile("xlsx", bytes)).items.find((item) =>
      item.id.endsWith("!B2"),
    )!;
    const operation = {
      type: "setCell",
      id: item.id,
      expect: item.text,
      value: { type: "number", value: 1 },
    };
    await expect(
      editFile(
        "xlsx",
        bytes,
        JSON.stringify({ operations: [operation, operation] }),
      ),
    ).rejects.toThrow("once");
  });

  it("edits slide text and order without rebuilding chart workbooks or masters", async () => {
    const content = JSON.parse(specification("pptx"));
    content.slides.push({
      title: "Second",
      master: "Review",
      elements: [
        { type: "text", x: 1, y: 1, width: 8, height: 1, text: "Second slide" },
      ],
    });
    const bytes = await createFile("pptx", JSON.stringify(content));
    const before = OfficePackage.open("pptx", bytes);
    const inspection = await inspectFile("pptx", bytes);
    const text = inspection.items.find(
      (item) =>
        item.kind === "text" && item.id.startsWith("ppt/slides/slide1.xml"),
    )!;
    const slides = inspection.items
      .filter((item) => item.kind === "slide")
      .map((item) => item.id);
    const result = await editFile(
      "pptx",
      bytes,
      JSON.stringify({
        operations: [
          {
            type: "replaceText",
            id: text.id,
            expect: text.text,
            text: "Updated claim",
          },
          { type: "reorderSlides", ids: slides.toReversed() },
        ],
      }),
    );
    const after = OfficePackage.open("pptx", result);
    for (const [name, value] of before.parts)
      if (!["ppt/slides/slide1.xml", "ppt/presentation.xml"].includes(name))
        expect(after.parts.get(name)).toEqual(value);
    expect(
      (await inspectFile("pptx", result)).items
        .filter((item) => item.kind === "slide")
        .map((item) => item.id),
    ).toEqual(slides.toReversed());
  });

  it("exports typed CSV without executing formula-like string cells", async () => {
    const bytes = await createFile(
      "xlsx",
      JSON.stringify({
        title: "CSV",
        sheets: [
          {
            name: "Data",
            cells: [
              {
                address: "A1",
                value: {
                  type: "string",
                  value: '=HYPERLINK("https://example.com")',
                },
              },
              { address: "B1", value: { type: "number", value: -12 } },
            ],
          },
        ],
      }),
    );
    expect(
      Buffer.from(exportCSV(bytes, "xl/worksheets/sheet1.xml")).toString(),
    ).toBe('"\'=HYPERLINK(""https://example.com"")","-12"\r\n');
    const formula = await createFile("xlsx", specification("xlsx"));
    expect(() => exportCSV(formula, "xl/worksheets/sheet1.xml")).toThrow(
      "calculated",
    );
  });

  it("preserves native numeric precision in inspection, editing preconditions and CSV", async () => {
    const bytes = await createFile(
      "xlsx",
      JSON.stringify({
        title: "Exact native numbers",
        sheets: [
          {
            name: "Data",
            cells: [1, 2, 3, 4].map((value) => ({
              address: `A${value}`,
              value: { type: "number", value },
            })),
          },
        ],
      }),
    );
    const archive = OfficePackage.open("xlsx", bytes);
    const part = "xl/worksheets/sheet1.xml";
    const decimals = [
      "9007199254740993",
      "1.0000000000000001",
      "-0",
      "1.234E+2",
    ];
    let xml = archive.xml(part).source;
    decimals.forEach((value, index) => {
      xml = xml.replace(`<v>${index + 1}</v>`, `<v>${value}</v>`);
    });
    archive.setXML(part, xml);
    const imported = archive.save();
    const inspection = await inspectFile("xlsx", imported);
    const first = inspection.items.find((item) => item.id.endsWith("!A1"))!;
    expect(JSON.parse(first.text)).toEqual({
      type: "number",
      value: decimals[0],
      representation: "native_decimal",
    });
    expect(
      JSON.parse(inspection.items.find((item) => item.id.endsWith("!A2"))!.text)
        .value,
    ).toBe(decimals[1]);
    expect(
      inspection.checks.some((check) => check.code === "native_decimal_text"),
    ).toBe(true);
    expect(Buffer.from(exportCSV(imported, part)).toString()).toBe(
      decimals.map((value) => `"${value}"\r\n`).join(""),
    );
    const revised = await editFile(
      "xlsx",
      imported,
      JSON.stringify({
        operations: [
          {
            type: "setCell",
            id: first.id,
            expect: first.text,
            value: { type: "string", value: decimals[0] },
          },
        ],
      }),
    );
    expect(
      JSON.parse(
        (await inspectFile("xlsx", revised)).items.find(
          (item) => item.id === first.id,
        )!.text,
      ),
    ).toEqual({ type: "string", value: decimals[0] });
    expect(OfficePackage.open("xlsx", revised).xml(part).source).toContain(
      `<v>${decimals[1]}</v>`,
    );
  });

  it("treats an empty formula cache as missing and refuses nonnumeric numeric caches", async () => {
    const bytes = await createFile(
      "xlsx",
      JSON.stringify({
        title: "Caches",
        sheets: [
          {
            name: "Data",
            cells: [{ address: "A1", value: { type: "formula", value: "=1" } }],
          },
        ],
      }),
    );
    const archive = OfficePackage.open("xlsx", bytes);
    const part = "xl/worksheets/sheet1.xml";
    const xml = archive.xml(part).source;
    archive.setXML(part, xml.replace("<f>1</f>", "<f>1</f><v/>"));
    expect(
      (await inspectFile("xlsx", archive.save())).facts.missingFormulaCaches,
    ).toBe(1);
    expect(() => exportCSV(archive.save(), part)).toThrow("calculated");
    archive.setXML(part, xml.replace("<f>1</f>", "<f>1</f><v>=1+1</v>"));
    await expect(inspectFile("xlsx", archive.save())).rejects.toThrow(
      "numeric spreadsheet cell",
    );
    expect(() => exportCSV(archive.save(), part)).toThrow(
      "numeric spreadsheet cell",
    );
  });

  it("refuses external-data formulas and out-of-revision edits", async () => {
    await expect(
      createFile(
        "xlsx",
        JSON.stringify({
          title: "Unsafe",
          sheets: [
            {
              name: "Data",
              cells: [
                {
                  address: "A1",
                  value: {
                    type: "formula",
                    value: '=WEBSERVICE("https://example.com")',
                  },
                },
              ],
            },
          ],
        }),
      ),
    ).rejects.toThrow("external-data");
    const bytes = await createFile("docx", specification("docx"));
    await expect(
      editFile(
        "docx",
        bytes,
        JSON.stringify({
          operations: [
            {
              type: "replaceText",
              id: "word/styles.xml#text:0",
              expect: "",
              text: "bad",
            },
          ],
        }),
      ),
    ).rejects.toThrow("identifier");
  });

  it("rejects duplicate JSON keys and numbers that lose authored precision", async () => {
    const content =
      '{"title":"Numbers","sheets":[{"name":"Data","cells":[{"address":"A1","value":{"type":"number","value":NUMBER}}]}]}';
    for (const number of ["9007199254740993", "1.0000000000000001"])
      await expect(
        createFile("xlsx", content.replace("NUMBER", number)),
      ).rejects.toThrow("exactly representable");
    await expect(
      createFile("xlsx", content.replace("NUMBER", "12.34")),
    ).resolves.toBeInstanceOf(Uint8Array);
    await expect(
      createFile("docx", '{"title":"one","title":"two","blocks":[]}'),
    ).rejects.toThrow();
  });
});

describe("native package admission", () => {
  it("rejects XML entity declarations, malformed nesting and processing instructions", () => {
    expect(() =>
      parseXML('<!DOCTYPE x [<!ENTITY x "secret">]><x>&x;</x>'),
    ).toThrow("document types");
    expect(() => parseXML("<x><y></x>")).toThrow("invalid XML");
    expect(() => parseXML("<?process malicious?><x/>")).toThrow("processing");
  });

  it("refuses path traversal and forged ZIP size before admission", async () => {
    expect(() =>
      encodeArchive(new Map([["../outside", Buffer.from("x")]])),
    ).toThrow("unsafe");
    const bytes = Buffer.from(await createFile("docx", specification("docx")));
    const central = bytes.indexOf(Buffer.from([0x50, 0x4b, 0x01, 0x02]));
    const offset = bytes.readUInt32LE(central + 42);
    bytes.writeUInt32LE(0, central + 24);
    bytes.writeUInt32LE(0, offset + 22);
    expect(() => OfficePackage.open("docx", bytes)).toThrow(/size|checksum/u);
  });

  it("enforces actual inflation bounds when a compressed entry lies about its size", () => {
    const small = Buffer.from(
      encodeArchive(new Map([["bomb", Buffer.from("x")]])),
    );
    const directory = small.indexOf(Buffer.from([0x50, 0x4b, 0x01, 0x02]));
    const compressed = deflateRawSync(
      Buffer.alloc(contentLimits.fileBytes + 1, 65),
    );
    const header = Buffer.from(small.subarray(0, 34));
    const central = Buffer.from(small.subarray(directory, directory + 50));
    const end = Buffer.from(small.subarray(directory + 50));
    header.writeUInt32LE(compressed.length, 18);
    central.writeUInt32LE(compressed.length, 20);
    end.writeUInt32LE(header.length + compressed.length, 16);
    expect(() =>
      OfficePackage.open(
        "docx",
        Buffer.concat([header, compressed, central, end]),
      ),
    ).toThrow(/larger than|maxOutputLength/u);
  });

  it("rejects duplicate ZIP identities even if local and central names agree", () => {
    const bytes = Buffer.from(
      encodeArchive(
        new Map([
          ["a", Buffer.from("one")],
          ["b", Buffer.from("two")],
        ]),
      ),
    );
    const first = bytes.indexOf(Buffer.from([0x50, 0x4b, 0x01, 0x02]));
    const second = first + 47;
    bytes[second + 46] = "a".charCodeAt(0);
    bytes[bytes.readUInt32LE(second + 42) + 30] = "a".charCodeAt(0);
    expect(() => OfficePackage.open("docx", bytes)).toThrow("duplicate");
  });

  it("rejects split external Word fields independently of namespace prefixes", async () => {
    const bytes = await createFile("docx", specification("docx"));
    const archive = OfficePackage.open("docx", bytes);
    const original = archive.xml("word/document.xml").source;
    const modified = original
      .replace(
        "<w:body>",
        "<w:body><w:p><w:r><w:instrText>DD</w:instrText></w:r><w:r><w:instrText>E external</w:instrText></w:r></w:p>",
      )
      .replace(/xmlns:w=/gu, "xmlns:word=")
      .replace(/\bw:/gu, "word:");
    archive.setXML("word/document.xml", modified);
    expect(() => OfficePackage.open("docx", archive.save())).toThrow(
      "external Word field",
    );
  });

  it("rejects automatic external relationships while retaining explicit safe hyperlinks", async () => {
    const bytes = await createFile("docx", specification("docx"));
    const archive = OfficePackage.open("docx", bytes);
    const path = "word/_rels/document.xml.rels";
    const xml = archive.xml(path);
    archive.setXML(
      path,
      xml.source.replace(
        "</Relationships>",
        '<Relationship Id="unsafe" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" TargetMode="External" Target="http://127.0.0.1/metadata"/></Relationships>',
      ),
    );
    expect(() => OfficePackage.open("docx", archive.save())).toThrow(
      "hyperlinks",
    );
  });

  it("rejects macro types, duplicate relationship IDs and missing targets", async () => {
    const bytes = await createFile("docx", specification("docx"));
    const macro = OfficePackage.open("docx", bytes);
    const types = macro.xml("[Content_Types].xml");
    macro.setXML(
      "[Content_Types].xml",
      types.source.replace(
        "wordprocessingml.document.main+xml",
        "ms-word.document.macroEnabled.main+xml",
      ),
    );
    expect(() => OfficePackage.open("docx", macro.save())).toThrow(
      "macro-free",
    );
    const missing = OfficePackage.open("docx", bytes);
    missing.parts.delete("word/styles.xml");
    expect(() => OfficePackage.open("docx", missing.save())).toThrow(
      "missing part",
    );
  });
});

describe("passive PDF edits", () => {
  it("rotates and reorders original pages and fills editable fields", async () => {
    const pdf = await PDFDocument.create();
    const first = pdf.addPage([612, 792]);
    first.setRotation(degrees(0));
    pdf.addPage([400, 500]);
    const field = pdf.getForm().createTextField("company");
    field.setText("Original");
    field.addToPage(first, {
      x: 40,
      y: 600,
      width: 200,
      height: 30,
      font: await pdf.embedFont(StandardFonts.Helvetica),
    });
    const bytes = await pdf.save({ useObjectStreams: false });
    const inspection = await inspectFile("pdf", bytes);
    const page = inspection.items.find((item) => item.id === "page:1")!;
    const form = inspection.items.find((item) => item.id === "field:company")!;
    const result = await editFile(
      "pdf",
      bytes,
      JSON.stringify({
        operations: [
          { type: "rotatePage", id: page.id, expect: page.text, degrees: 90 },
          {
            type: "fillField",
            id: form.id,
            expect: form.text,
            value: "Updated",
          },
          { type: "reorderPages", ids: ["page:2", "page:1"] },
        ],
      }),
    );
    const after = await inspectFile("pdf", result);
    expect(after.items.find((item) => item.id === "page:2")?.text).toContain(
      '"rotation":90',
    );
    expect(
      after.items.find((item) => item.id === "field:company")?.text,
    ).toContain("Updated");
  });

  it("refuses compressed objects and active actions before editing", async () => {
    const pdf = await PDFDocument.create();
    pdf.addPage();
    await expect(inspectFile("pdf", await pdf.save())).rejects.toThrow(
      /ObjStm|XRef/u,
    );
    const raw = Buffer.from(
      await pdf.save({ useObjectStreams: false }),
    ).toString("latin1");
    await expect(
      inspectFile(
        "pdf",
        Buffer.from(
          raw.replace(
            "/Type /Catalog",
            "/Type /Catalog /OpenAction << /S /JavaScript /JS (bad) >>",
          ),
          "latin1",
        ),
      ),
    ).rejects.toThrow("not admitted");
  });
});
