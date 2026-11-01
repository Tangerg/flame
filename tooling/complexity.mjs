// Ratchets cyclomatic complexity in authored TypeScript and JavaScript the
// same way tooling/complexity does for Go: a function above the threshold must
// be listed at no more than its recorded score, and the list only shrinks.
import { readFile, writeFile } from "node:fs/promises";
import { readFileSync } from "node:fs";
import { parseSync, traverse } from "@babel/core";
import { pathToFileURL } from "node:url";
import { delimiter, join, resolve } from "node:path";
import { root, run } from "./workspace.mjs";

const threshold = 20;
const baselinePath = join(root, "tooling/complexity-baseline.txt");
export function authoredSources(directory) {
  const paths = run(
    "git",
    ["ls-files", "-z", "--cached", "--others", "--exclude-standard"],
    directory,
    process.env,
    true,
  );
  const testOrProjection =
    /(?:^|\/)(?:node_modules|out|testdata|fixtures|__tests__|test|tests|testing|generated|package)\/|\.(?:test|spec|generated|d)\.[^/]+$/;
  return paths
    .toString()
    .split("\0")
    .filter(
      (path) => /\.[cm]?[jt]sx?$/.test(path) && !testOrProjection.test(path),
    );
}

function classScopes(directory, filename) {
  const source = readFileSync(join(directory, filename), "utf8");
  const plugins = [];
  if (/\.[cm]?tsx?$/.test(filename)) plugins.push("typescript");
  if (filename.endsWith("sx")) plugins.push("jsx");
  const ast = parseSync(source, {
    filename,
    babelrc: false,
    configFile: false,
    sourceType: "unambiguous",
    parserOpts: { plugins },
  });
  const scopes = [];
  traverse(ast, {
    Class(path) {
      const node = path.node;
      const name = node.id?.name ?? path.parentPath.node.id?.name;
      if (name)
        scopes.push({
          name,
          start: Buffer.byteLength(source.slice(0, node.start)),
          end: Buffer.byteLength(source.slice(0, node.end)),
        });
    },
  });
  return scopes;
}

export function measure(directory = root) {
  const sources = authoredSources(directory);
  const scores = new Map();
  const ordinals = new Map();
  const scopes = new Map();
  // Keep the command below the platform argument limit, including on macOS.
  const batchSize = 100;
  for (let start = 0; start < sources.length; start += batchSize) {
    const output = run(
      "oxlint",
      [
        "-A",
        "all",
        "-W",
        "eslint/complexity",
        "--disable-nested-config",
        "--no-ignore",
        "--format",
        "json",
        ...sources.slice(start, start + batchSize),
      ],
      directory,
      {
        ...process.env,
        PATH: join(root, "node_modules/.bin") + delimiter + process.env.PATH,
      },
      true,
    );
    const diagnostics = JSON.parse(output).diagnostics.sort(
      (left, right) => left.labels[0].span.offset - right.labels[0].span.offset,
    );
    for (const diagnostic of diagnostics) {
      const match = /(?:`([^`]*)` )?has a complexity of (\d+)/.exec(
        diagnostic.message,
      );
      if (!match)
        throw new Error(
          `complexity: unexpected diagnostic ${diagnostic.message}`,
        );
      const file = diagnostic.filename;
      if (!scopes.has(file)) scopes.set(file, classScopes(directory, file));
      const offset = diagnostic.labels[0].span.offset;
      const scope = scopes
        .get(file)
        .filter((scope) => scope.start <= offset && offset < scope.end)
        .map((scope) => scope.name);
      const symbol = [...scope, match[1] ?? "<anonymous>"].join(".");
      const key = file + ":" + symbol;
      const ordinal = (ordinals.get(key) ?? 0) + 1;
      ordinals.set(key, ordinal);
      scores.set(ordinal === 1 ? key : `${key}#${ordinal}`, Number(match[2]));
    }
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

async function main() {
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
}

if (
  process.argv[1] &&
  pathToFileURL(resolve(process.argv[1])).href === import.meta.url
)
  await main();
