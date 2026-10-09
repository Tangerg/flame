import { test } from "node:test";
import assert from "node:assert/strict";
import { createElement } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { readFile } from "node:fs/promises";
import { rolldown } from "rolldown";
import * as glyphs from "@flame/icons/react";
import { icons } from "@flame/icons/catalog";
import { readSvg } from "../scripts/svg.mjs";

test("all published components select optical masters from their rendered size", () => {
  for (const { component } of icons) {
    const Icon = glyphs[component];
    for (const size of [
      ...Array.from({ length: 28 }, (_, i) => i + 9),
      21.99,
      22,
      22.01,
    ]) {
      const svg = renderToStaticMarkup(createElement(Icon, { size }));
      const grid = size < 22 ? 16 : 24;
      assert.match(svg, new RegExp(`viewBox="0 0 ${grid} ${grid}"`));
      assert.ok(svg.includes(`width="${size}" height="${size}"`));
    }
  }
});

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

test("size and authored geometry have one owner even with untyped props", () => {
  const svg = renderToStaticMarkup(
    createElement(glyphs.Eye, {
      size: 24,
      viewBox: "0 0 50 50",
      strokeWidth: 10,
      fill: "blue",
      children: "wrong",
      style: { width: 80, height: 90, opacity: 0.5 },
    }),
  );
  assert.match(svg, /viewBox="0 0 24 24"/);
  assert.match(svg, /stroke-width="1.5"/);
  assert.match(svg, /fill="none"/);
  assert.match(svg, /width:24px;height:24px;opacity:0.5/);
  assert.doesNotMatch(svg, /wrong/);
  for (const size of [0, -1, NaN, Infinity, "24"]) {
    assert.throws(
      () => renderToStaticMarkup(createElement(glyphs.Eye, { size })),
      RangeError,
    );
  }
});

test("SVG downloads and React components carry the same paths and weights", async () => {
  for (const icon of icons) {
    for (const grid of [16, 24]) {
      const text = await readFile(
        new URL(`../dist/${icon.svg[grid]}`, import.meta.url),
        "utf8",
      );
      const paths = [
        ...text.matchAll(/<path d="([^"]+)"(?: stroke-width="([^"]+)")?\/>/g),
      ];
      const rendered = renderToStaticMarkup(
        createElement(glyphs[icon.component], { size: grid }),
      );
      assert.equal([...rendered.matchAll(/<path /g)].length, paths.length);
      for (const [, d, weight] of paths)
        assert.ok(
          rendered.includes(
            `d="${d}"${weight ? ` stroke-width="${weight}"` : ""}`,
          ),
        );
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
