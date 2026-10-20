import { readFile, writeFile, mkdir, rm, readdir, cp } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import { join } from "node:path";
import { families } from "../catalog.mjs";
import { readSvg, writeSvg } from "./svg.mjs";

const root = fileURLToPath(new URL("../", import.meta.url));
const dist = join(root, "dist");
const glyphs = [];
const masters = new Map();
const expectedSources = new Set([
  "slash",
  ...families.flatMap((f) => [
    f.source ?? f.name,
    ...(f.base ? [f.base] : []),
    ...(f.modifier ? [f.modifier] : []),
  ]),
]);
for (const grid of [16, 24]) {
  const directory = join(root, "source", String(grid));
  const files = await readdir(directory);
  const expected = [...expectedSources].map((name) => `${name}.svg`).sort();
  if (JSON.stringify(files.sort()) !== JSON.stringify(expected))
    throw new Error(`${directory}: source files and catalog disagree`);
  for (const name of expectedSources) {
    const file = join(directory, `${name}.svg`);
    const master = readSvg(await readFile(file, "utf8"), grid, file);
    if (master.strokeWidth !== (grid === 16 ? 1.25 : 1.5))
      throw new Error(`${file}: master stroke weight drifted`);
    masters.set(`${grid}/${name}`, master);
  }
}
for (const family of families) {
  glyphs.push({ ...family, source: family.source ?? family.name });
  if (family.offLabel !== undefined)
    glyphs.push({
      ...family,
      name: `${family.name}-off`,
      component: `${family.component}Off`,
      label: family.offLabel,
      source: family.name,
      modifier: "slash",
    });
}
for (const field of ["name", "component"]) {
  if (new Set(glyphs.map((g) => g[field])).size !== glyphs.length)
    throw new Error(`duplicate ${field}`);
}
// Generated modules are outside the authoring directory and are replaced as a unit.
await rm(dist, { recursive: true, force: true });
await rm(join(root, "react/generated"), { recursive: true, force: true });
await mkdir(join(root, "react/generated"), { recursive: true });
await mkdir(join(dist, "svg/16"), { recursive: true });
await mkdir(join(dist, "svg/24"), { recursive: true });
for (const glyph of glyphs) {
  const pair = [16, 24].map((grid) => {
    const base = masters.get(`${grid}/${glyph.source}`);
    return {
      ...base,
      paths: [
        ...(glyph.base ? masters.get(`${grid}/${glyph.base}`).paths : []),
        ...base.paths,
        ...(glyph.modifier
          ? masters.get(`${grid}/${glyph.modifier}`).paths
          : []),
      ],
    };
  });
  for (const master of pair)
    await writeFile(
      join(dist, `svg/${master.grid}/${glyph.name}.svg`),
      writeSvg(master),
    );
  await writeFile(
    join(root, `react/generated/${glyph.name}.ts`),
    `import { createIcon } from "../createIcon.js";\nexport const ${glyph.component} = /* @__PURE__ */ createIcon(${JSON.stringify(glyph.component)}, ${JSON.stringify(pair[0])}, ${JSON.stringify(pair[1])});\n`,
  );
}
await writeFile(
  join(root, "react/generated/index.ts"),
  `export type { IconProps, IconComponent } from "../createIcon.js";\n${glyphs.map((g) => `export { ${g.component} } from "./${g.name}.js";`).join("\n")}\n`,
);
const catalog = glyphs.map(({ name, component, label, tags, group }) => ({
  name,
  component,
  label,
  tags,
  group,
  svg: { 16: `svg/16/${name}.svg`, 24: `svg/24/${name}.svg` },
}));
await writeFile(
  join(dist, "catalog.js"),
  `export const icons = ${JSON.stringify(catalog, null, 2)};\n`,
);
await writeFile(
  join(dist, "catalog.d.ts"),
  "export declare const icons: readonly { readonly name: string; readonly component: string; readonly label: string; readonly tags: readonly string[]; readonly group: string; readonly svg: { readonly 16: string; readonly 24: string } }[];\n",
);
await cp(join(root, "preview"), join(dist, "preview"), { recursive: true });
console.log(`Built ${glyphs.length} icons with 16px and 24px optical masters.`);
