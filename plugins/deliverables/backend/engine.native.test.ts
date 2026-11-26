import { mkdtemp, realpath, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import PptxGenJS from "pptxgenjs";
import { expect, it } from "vitest";
import { Engine } from "./engine";
import { OfficePackage } from "./formats/archive";
import { cellRange } from "./formats/workbook-cells";
import {
  catalog,
  createFile,
  editFile,
  exportCSV,
  inspectFile,
  layoutFile,
} from "./formats";
import { LIMITS } from "../contracts/limits";

// Explicit host qualification uses the same production owner and confinement.
// Ordinary unit runs do not assume LibreOffice/Poppler are installed or permitted.
const native = it.runIf(process.env.FLAME_DELIVERABLES_NATIVE_TESTS === "1");

it("repairs a legacy embedded chart table when editing its title and legend", async () => {
  const presentation = new PptxGenJS();
  presentation
    .addSlide()
    .addChart(
      presentation.ChartType.bar,
      [{ name: "East", labels: ["Q1", "Q2"], values: [39, 43] }],
      {
        x: 1,
        y: 1,
        w: 8,
        h: 4,
        showTitle: true,
        title: "Original",
        showLegend: true,
        legendPos: "b",
      },
    );
  const original = await presentation.write({
    outputType: "uint8array",
    compression: true,
  });
  if (!(original instanceof Uint8Array))
    throw new Error("presentation encoder returned an unexpected result");
  const tablePart = "xl/tables/table1.xml";
  const embedded = (bytes: Uint8Array): OfficePackage => {
    const bundle = OfficePackage.open("pptx", bytes);
    const bytesInPart = [...bundle.parts.entries()].find(([name]) =>
      name.endsWith(".xlsx"),
    )?.[1];
    if (!bytesInPart) throw new Error("chart fixture has no embedded workbook");
    return OfficePackage.open("xlsx", bytesInPart);
  };
  const originalBook = embedded(original);
  expect(originalBook.xml(tablePart).root.attributes.ref).toBe("A1:B3'");

  const chart = layoutFile(original).slides[0]!.elements.find(
    (element) => element.kind === "chart",
  )!;
  const edited = await editFile(
    "pptx",
    original,
    JSON.stringify({
      operations: [
        {
          type: "replaceText",
          id: chart.text[0]!.id,
          expect: "Original",
          text: "Updated",
        },
        {
          type: "setChartLegend",
          id: chart.id,
          expect: chart.chart!.legendPosition,
          position: "t",
        },
      ],
    }),
  );

  const editedBook = embedded(edited);
  expect(editedBook.xml(tablePart).root.attributes.ref).toBe("A1:B3");
  expect(editedBook.parts.get("xl/worksheets/sheet1.xml")).toEqual(
    originalBook.parts.get("xl/worksheets/sheet1.xml"),
  );
  expect(embedded(original).xml(tablePart).root.attributes.ref).toBe("A1:B3'");
  const updated = layoutFile(edited).slides[0]!.elements.find(
    (element) => element.id === chart.id,
  )!;
  expect(updated.text[0]!.text).toBe("Updated");
  expect(updated.chart?.legendPosition).toBe("t");
  expect(updated.chart?.data).toEqual(chart.chart?.data);
});

native.each(["docx", "xlsx", "pptx"] as const)(
  "renders the catalog %s with the real confined toolchain",
  async (format) => {
    const data = await mkdtemp(
      join(await realpath(tmpdir()), "flame-deliverables-native-"),
    );
    const engine = new Engine(process.env, data);
    try {
      const example = catalog().formats.find(
        (entry) => entry.format === format,
      )?.exampleJSON;
      if (!example) throw new Error("catalog example missing");
      const bytes = await createFile(format, example);
      const result = await engine.render(
        format,
        bytes,
        new AbortController().signal,
      );
      expect(result.report.pages.length).toBeGreaterThan(0);
      expect(
        result.report.engines.some((entry) => entry.name === "LibreOffice"),
      ).toBe(true);
      expect(
        result.files.filter((file) => file.mime === "image/png"),
      ).toHaveLength(result.report.pages.length);
      expect(result.report.extractedText.trim()).not.toBe("");
      // Text that only the catalog's chart draws; a blank chart frame omits it.
      const chartText = {
        docx: undefined,
        xlsx: "Quarterly revenue",
        pptx: "Revenue (USD million)",
      }[format];
      if (chartText) expect(result.report.extractedText).toContain(chartText);
      expect(result.report.fontStatus).toBe("unverified");
    } finally {
      await engine.close();
      await rm(data, { recursive: true, force: true });
    }
  },
  LIMITS.processMs + 10_000,
);

native(
  "recalculates the catalog workbook with real LibreOffice and observes calculated values",
  async () => {
    const data = await mkdtemp(
      join(await realpath(tmpdir()), "flame-deliverables-native-"),
    );
    const engine = new Engine(process.env, data);
    try {
      const example = catalog().formats.find(
        (entry) => entry.format === "xlsx",
      )?.exampleJSON;
      if (!example) throw new Error("catalog workbook example missing");
      const input = await createFile("xlsx", example);
      const result = await engine.recalculate(
        input,
        new AbortController().signal,
      );
      const inspection = await inspectFile("xlsx", result.bytes);
      expect(inspection.facts.missingFormulaCaches).toBe(0);
      expect(inspection.facts.formulaErrors).toBe(0);
      expect(
        Buffer.from(
          exportCSV(result.bytes, "xl/worksheets/sheet1.xml"),
        ).toString("utf8"),
      ).toContain("212");
    } finally {
      await engine.close();
      await rm(data, { recursive: true, force: true });
    }
  },
  LIMITS.processMs + 10_000,
);

native(
  "renders a presentation after element, chart-data and slide edits with the real confined toolchain",
  async () => {
    const data = await mkdtemp(
      join(await realpath(tmpdir()), "flame-deliverables-native-"),
    );
    const engine = new Engine(process.env, data);
    try {
      const created = await createFile(
        "pptx",
        JSON.stringify({
          title: "Edited review",
          slides: [
            {
              title: "Revenue",
              elements: [
                {
                  type: "text",
                  x: 0.5,
                  y: 0.5,
                  width: 9,
                  height: 1,
                  text: "Revenue grew",
                },
                {
                  type: "chart",
                  chart: "bar",
                  title: "Revenue by region",
                  x: 1,
                  y: 2,
                  width: 8,
                  height: 4,
                  series: [
                    {
                      name: "Revenue (USD million)",
                      labels: ["Q1", "Q2"],
                      values: [100, 112],
                    },
                  ],
                },
              ],
              notes: "Explain the growth",
            },
            {
              title: "Appendix",
              elements: [
                {
                  type: "text",
                  x: 0.5,
                  y: 0.5,
                  width: 9,
                  height: 1,
                  text: "Appendix detail",
                },
              ],
            },
          ],
        }),
      );
      const [first, second] = layoutFile(created).slides;
      const bundle = OfficePackage.open("pptx", created);
      const workbooks = [...bundle.parts.entries()].filter(([part]) =>
        part.endsWith(".xlsx"),
      );
      expect(workbooks).toHaveLength(1);
      const workbook = OfficePackage.open("xlsx", workbooks[0]![1]);
      for (const part of workbook.parts.keys()) {
        if (/^xl\/tables\/[^/]+\.xml$/u.test(part))
          expect(() =>
            cellRange(workbook.xml(part).root.attributes.ref!),
          ).not.toThrow();
      }
      const title = first!.elements.find((element) => element.kind === "text")!;
      const chart = first!.elements.find(
        (element) => element.kind === "chart",
      )!;
      const edited = await editFile(
        "pptx",
        created,
        JSON.stringify({
          operations: [
            {
              type: "setFrame",
              id: title.id,
              expect: title.frame,
              frame: { ...title.frame!, y: title.frame!.y + 457200 },
            },
            {
              type: "replaceText",
              id: title.text[0]!.id,
              expect: "Revenue grew",
              text: "Revenue grew 12%",
            },
            {
              type: "setChartLegend",
              id: chart.id,
              expect: chart.chart!.legendPosition,
              position: "r",
            },
            {
              type: "replaceText",
              id: chart.text[0]!.id,
              expect: "Revenue by region",
              text: "Revenue by territory",
            },
            {
              type: "setChartData",
              id: chart.id,
              expect: chart.chart!.data,
              data: {
                categories: ["North", "South"],
                series: [{ name: "Converted", values: [720, 806.4] }],
              },
            },
          ],
        }),
      );
      const restructured = await editFile(
        "pptx",
        edited,
        JSON.stringify({
          operations: [
            { type: "duplicateSlide", id: first!.id },
            { type: "deleteSlide", id: second!.id },
          ],
        }),
      );
      const result = await engine.render(
        "pptx",
        restructured,
        new AbortController().signal,
      );
      expect(result.report.pages).toHaveLength(2);
      expect(
        result.report.checks.filter((check) => check.severity === "error"),
      ).toEqual([]);
      const text = result.report.extractedText;
      expect(text.split("Revenue grew 12%")).toHaveLength(3);
      for (const value of [
        "North",
        "South",
        "Converted",
        "Revenue by territory",
      ])
        expect(text.split(value)).toHaveLength(3);
      expect(text).not.toContain("Q1");
      expect(text).not.toContain("Appendix detail");
    } finally {
      await engine.close();
      await rm(data, { recursive: true, force: true });
    }
  },
  LIMITS.processMs + 10_000,
);
