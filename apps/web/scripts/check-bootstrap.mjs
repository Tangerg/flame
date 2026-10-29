#!/usr/bin/env node
import { readFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { checkPresentation } from "../../../tooling/presentation.mjs";

const repository = fileURLToPath(new URL("../../../", import.meta.url));
await checkPresentation(repository);
const read = (path) => readFile(new URL(path, import.meta.url), "utf8");
const [html, visualHtml, modality, documentCss] = await Promise.all([
  read("../index.html"),
  read("../visual/index.html"),
  read("../public/focus-modality.js"),
  read("../../../packages/ui/src/globals.css"),
]);
const declarations = [...modality.matchAll(/setAttribute\("(data-[a-z-]+)",\s*"([^"]+)"\)/g)];
const attributes = new Set(declarations.map((match) => match[1]));
const modes = new Set(declarations.map((match) => match[2]));
const attribute = attributes.size === 1 ? declarations[0]?.[1] : undefined;
if (
  !attribute ||
  modes.size !== 2 ||
  !modes.has("pointer") ||
  !modes.has("keyboard") ||
  !documentCss.includes(`html:not([${attribute}])`) ||
  !documentCss.includes(`html[${attribute}="pointer"]`) ||
  !documentCss.includes(`html[${attribute}="keyboard"]`)
)
  throw new Error("the input-modality owner and shared focus stylesheet disagree");
for (const entry of [html, visualHtml])
  if (!entry.includes('src="/focus-modality.js"'))
    throw new Error("both Web entries must load the input-modality owner");
console.log("[check-bootstrap] presentation projections and input modality agree");
