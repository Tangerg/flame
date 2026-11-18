import { test } from "node:test";
import assert from "node:assert/strict";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { readFile } from "node:fs/promises";
import { rolldown } from "rolldown";
import * as glyphs from "@flame/icons/react";
import { icons } from "@flame/icons/catalog";
import { readSvg } from "../scripts/svg.mjs";
test("decorative defaults, native accessibility and attributes survive rendering", () => {
  const decorative = renderToStaticMarkup(createElement(glyphs.Bell));
  assert.match(decorative, /aria-hidden="true"/);
  assert.match(decorative, /width="16"/);
  const labelled = renderToStaticMarkup(
    createElement(glyphs.BellOff, {
      "aria-label": "Muted",
      className: "custom",
      "data-test": "bell",
      color: "red",
    }),
  );
  assert.match(labelled, /role="img"/);
  assert.doesNotMatch(labelled, /aria-hidden="true"/);
  assert.match(labelled, /data-test="bell"/);
  assert.match(labelled, /class="custom"/);
  assert.match(labelled, /stroke="currentColor"/);
  assert.match(labelled, /color="red"/);
});

test("SVG downloads and React components carry the same paths, weights and paint", async () => {
  for (const icon of icons) {
    for (const grid of [16, 24]) {
      const text = await readFile(
        new URL(`../dist/${icon.svg[grid]}`, import.meta.url),
        "utf8",
      );
      const paths = [...text.matchAll(/<path ([^>]+)\/>/g)];
      const rendered = renderToStaticMarkup(
        createElement(glyphs[icon.component], { size: grid }),
      );
      assert.equal([...rendered.matchAll(/<path /g)].length, paths.length);
      for (const [, attributes] of paths)
        assert.ok(rendered.includes(`<path ${attributes}`));
      assert.doesNotMatch(text, /mask|filter|clipPath|<image/);
    }
  }
});

test("source parser refuses unsupported or unsafe SVG features", () => {
  const root =
    '<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 16 16" fill="none" stroke="currentColor" stroke-width="1.25" stroke-linecap="round" stroke-linejoin="round">';
  for (const drawing of [
    "<script/>",
    '<path d="M0 0" onclick="run()"/>',
    '<path d="M0 0"/><image/>',
    '<path d="M0 0" stroke-width="NaN"/>',
    '<path d="M0 0" fill="red"/>',
    '<path d="M0 0" fill="url(#paint)"/>',
    '<path d="&quot;"/>',
  ]) {
    assert.throws(() => readSvg(`${root}${drawing}</svg>`, 16, "test.svg"));
  }
});

test("one named import excludes unrelated drawings and the catalog", async () => {
  const bundle = await rolldown({
    input: "single-icon",
    external: ["react"],
    plugins: [
      {
        name: "single-icon",
        resolveId: (id) => (id === "single-icon" ? id : null),
        load: (id) =>
          id === "single-icon"
            ? 'export { BellOff } from "@flame/icons/react";'
            : null,
      },
    ],
  });
  try {
    const { output } = await bundle.generate({ format: "esm" });
    const code = output.map((chunk) => chunk.code ?? "").join("\n");
    assert.ok(code.includes('"BellOff"'));
    for (const icon of icons.filter((icon) => icon.component !== "BellOff"))
      assert.ok(!code.includes(`"${icon.component}"`));
    assert.ok(!code.includes("notification"));
  } finally {
    await bundle.close();
  }
});
