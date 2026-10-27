import assert from "node:assert/strict";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { test } from "node:test";
import { backendTests } from "./test-backends.mjs";

test("backend test discovery includes new plugins and shared contracts", async () => {
  const directory = await mkdtemp(join(tmpdir(), "flame-backend-tests-"));
  try {
    for (const path of [
      "reports/backend/archive.test.ts",
      "presentation/backend/present.spec.ts",
      "presentation/contracts/nested/document.test.ts",
      "presentation/backend/start.ts",
      "presentation/component/renderer.test.tsx",
      "presentation/package/components/component.test.js",
      "scripts/build.test.mjs",
    ]) {
      const file = join(directory, path);
      await mkdir(dirname(file), { recursive: true });
      await writeFile(file, "");
    }
    assert.deepEqual(await backendTests(directory), [
      "presentation/backend/present.spec.ts",
      "presentation/contracts/nested/document.test.ts",
      "reports/backend/archive.test.ts",
    ]);
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});

test("backend test discovery preserves filesystem failures", async () => {
  const directory = await mkdtemp(join(tmpdir(), "flame-backend-tests-"));
  try {
    await mkdir(join(directory, "invalid"));
    await writeFile(join(directory, "invalid/backend"), "not a directory");
    await assert.rejects(backendTests(directory), { code: "ENOTDIR" });
  } finally {
    await rm(directory, { recursive: true, force: true });
  }
});
