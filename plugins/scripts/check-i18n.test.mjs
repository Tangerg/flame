import { execFileSync } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import { test } from "node:test";
import { strict as assert } from "node:assert";

test("localization checks production components while allowing test fixture prose", async () => {
  const root = await mkdtemp(join(tmpdir(), "flame-plugin-i18n-"));
  try {
    const component = join(root, "sample/component");

    await mkdir(join(component, "locales"), { recursive: true });
    const script = join(
      dirname(fileURLToPath(import.meta.url)),
      "check-i18n.mjs",
    );
    for (const locale of ["en", "zh", "zh-TW", "ja", "ko", "es", "fr", "de"])
      await writeFile(
        join(component, `locales/${locale}.ts`),
        'export const catalog = { save: "Save" };',
      );
    await writeFile(
      join(component, "component.tsx"),
      '<button>{t("save")}</button>',
    );
    await writeFile(
      join(component, "component.test.tsx"),
      '<input aria-label="Later draft" />',
    );
    await writeFile(
      join(component, "component.spec.tsx"),
      "<p>Fixture only</p>",
    );
    assert.match(
      execFileSync(process.execPath, [script, root], { encoding: "utf8" }),
      /complete/,
    );

    await writeFile(
      join(component, "component.tsx"),
      '<input aria-label="Product draft" />',
    );
    assert.throws(
      () => execFileSync(process.execPath, [script, root], { stdio: "pipe" }),
      (error) =>
        error.status === 1 &&
        error.stderr.toString().includes('aria-label="Product draft"'),
    );
    await mkdir(join(component, "nested"));
    await writeFile(
      join(component, "nested", "action.tsx"),
      'const url="https://example.test"; t("missing")',
    );
    await writeFile(join(component, "component.tsx"), 't("save")');
    assert.throws(
      () => execFileSync(process.execPath, [script, root], { stdio: "pipe" }),
      (error) =>
        error.status === 1 && error.stderr.toString().includes('t("missing")'),
    );
  } finally {
    await rm(root, { recursive: true, force: true });
  }
});
