// Ratchets cyclomatic complexity in authored TypeScript and JavaScript the
// same way tooling/complexity does for Go: a function above the threshold must
// be listed at no more than its recorded score, and the list only shrinks.
import { readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { root, run } from "./workspace.mjs";

const threshold = 20;
const baselinePath = join(root, "tooling/complexity-baseline.txt");
// Authored sources only; bundles and generated contracts are excluded by the
// generated-file ignore patterns oxlint already reads.
const sources = [
  "apps/web/src",
  "packages/backend-sdk/src",
  "packages/component-sdk/src",
  "packages/runtime-client/src",
  "packages/ui/src",
  "plugins",
];

function measure() {
  const output = run(
    "npx",
    [
      "oxlint",
      "-A",
      "all",
      "-W",
      "eslint/complexity",
      "--ignore-pattern",
      "**/package/**",
      "--ignore-pattern",
      "**/*.generated.*",
      "--ignore-pattern",
      "**/*.test.*",
      "--format",
      "json",
      ...sources,
    ],
    root,
    process.env,
    true,
  );
  return parse(output.toString());
}

function parse(json) {
  const scores = new Map();
  const anonymous = new Map();
  const diagnostics = JSON.parse(json).diagnostics.sort(
    (left, right) =>
      left.filename.localeCompare(right.filename) ||
      left.labels[0].span.offset - right.labels[0].span.offset,
  );
  for (const diagnostic of diagnostics) {
    const match = /(?:`([^`]*)` )?has a complexity of (\d+)/.exec(
      diagnostic.message,
    );
    if (!match)
      throw new Error(
        `complexity: unexpected diagnostic ${diagnostic.message}`,
      );
    // Anonymous hotspots are numbered within their file: a line number would
    // shift with every unrelated edit above them.
    let name = match[1];
    if (!name) {
      const ordinal = (anonymous.get(diagnostic.filename) ?? 0) + 1;
      anonymous.set(diagnostic.filename, ordinal);
      name = `<anonymous ${ordinal}>`;
    }
    scores.set(`${diagnostic.filename}:${name}`, Number(match[2]));
  }
  return scores;
}

async function readBaseline() {
  const accepted = new Map();
  for (const line of (await readFile(baselinePath, "utf8")).split("\n")) {
    if (!line.trim() || line.startsWith("#")) continue;
    const space = line.indexOf(" ");
    accepted.set(line.slice(space + 1), Number(line.slice(0, space)));
  }
  return accepted;
}

const current = measure();
if (process.argv.includes("--write")) {
  const lines = [...current.keys()]
    .sort()
    .map((key) => `${current.get(key)} ${key}`);
  await writeFile(
    baselinePath,
    `# Functions accepted above the complexity threshold, written by tooling/complexity.mjs --write.\n# This list may only shrink: reduce a function, then lower or remove its line.\n${lines.join("\n")}\n`,
  );
} else {
  const accepted = await readBaseline();
  const problems = [];
  for (const [key, score] of current) {
    const limit = accepted.get(key);
    if (limit === undefined)
      problems.push(
        `${key} has complexity ${score} (> ${threshold}); simplify it`,
      );
    else if (score > limit)
      problems.push(`${key} grew from ${limit} to ${score}`);
    else if (score < limit)
      problems.push(
        `${key} improved from ${limit} to ${score}; lower its baseline`,
      );
  }
  for (const key of accepted.keys()) {
    if (!current.has(key))
      problems.push(
        `${key} is no longer above ${threshold}; remove it from the baseline`,
      );
  }
  if (problems.length) {
    console.error(
      `complexity: ${problems.length} problem(s):\n  ${problems.join("\n  ")}`,
    );
    process.exit(1);
  }
}
