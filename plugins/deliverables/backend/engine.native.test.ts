import { mkdtemp, realpath, rm } from "node:fs/promises";
import { join } from "node:path";
import { tmpdir } from "node:os";
import { expect, it } from "vitest";
import { Engine } from "./engine";
import { catalog, createFile, exportCSV, inspectFile } from "./formats";
import { LIMITS } from "../contracts/limits";

// Explicit host qualification uses the same production owner and confinement.
// Ordinary unit runs do not assume LibreOffice/Poppler are installed or permitted.
const native = it.runIf(process.env.FLAME_DELIVERABLES_NATIVE_TESTS === "1");

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
