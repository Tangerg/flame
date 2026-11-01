import { strict as assert } from "node:assert";
import { execFileSync } from "node:child_process";
import { mkdir, mkdtemp, rm, writeFile } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { measure } from "./complexity.mjs";

test("the ratchet discovers authored sources and retains same-name methods in independent classes", async (t) => {
  const directory = await mkdtemp(join(tmpdir(), "flame-complexity-"));
  t.after(() => rm(directory, { recursive: true, force: true }));
  execFileSync("git", ["init", "-q"], { cwd: directory });
  const source = "new-workspace/policy.ts";
  const branch = "if (value > 0) value--;\n";
  const fixture =
    "/* 語 */" +
    ["First", "Second"]
      .map(
        (name, index) =>
          `class ${name} { check(value: number) {\n${branch.repeat(21 + index)}return value;\n} }\n`,
      )
      .join("")
      .replaceAll("\n", " ");
  await mkdir(join(directory, "new-workspace"));
  await writeFile(join(directory, source), fixture);
  await writeFile(join(directory, "new-workspace/unit.test.ts"), fixture);
  assert.deepEqual(
    [...measure(directory)],
    [
      [source + ":First.check", 22],
      [source + ":Second.check", 23],
    ],
  );
});
