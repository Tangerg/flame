import assert from "node:assert/strict";
import { test } from "node:test";
import { mkdtemp, writeFile, readFile, rm } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";

test("packed UI and icons work without the host or a StyleX compiler", async () => {
  const dir = await mkdtemp(path.join(tmpdir(), "flame-ui-consumer-"));
  try {
    const archive = (url) => {
      const packed = JSON.parse(
        execFileSync(
          "npm",
          ["pack", "--ignore-scripts", "--json", "--pack-destination", dir],
          { cwd: fileURLToPath(url), encoding: "utf8" },
        ),
      );
      return path.join(dir, packed[0].filename);
    };
    const icons = archive(new URL("../../icons", import.meta.url));
    const ui = archive(new URL("../", import.meta.url));
    await writeFile(
      path.join(dir, "package.json"),
      JSON.stringify({
        private: true,
        type: "module",
        dependencies: {
          "@flame/ui": `file:${ui}`,
          "@flame/icons": `file:${icons}`,
          react: "^19.3.0",
          "react-dom": "^19.3.0",
        },
      }),
    );
    execFileSync(
      "npm",
      ["install", "--offline", "--ignore-scripts", "--no-audit", "--no-fund"],
      { cwd: dir, stdio: "pipe" },
    );
    await writeFile(
      path.join(dir, "consumer.mjs"),
      `import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { Button, ContentVisibility, DiffStat, FontScaleProvider, IconButton, Segmented, Slider, Switch, TooltipProvider, useContentVisible } from '@flame/ui';
import { Brain } from '@flame/icons/react';
function Visibility() { return createElement('output', null, useContentVisible() ? 'content-visible' : 'content-hidden'); }
console.log(renderToStaticMarkup(createElement(FontScaleProvider, {fontSize: 18}, createElement(TooltipProvider, null, createElement(Button, null, 'Review'), createElement(DiffStat, {added: 18, removed: 6}), createElement(IconButton, {icon: Brain, 'aria-label': 'Memory'}), createElement(Segmented, {value: 'list', ariaLabel: 'View mode', options: [{value: 'list', label: 'List'}, {value: 'grid', label: 'Grid'}], onChange() {}}), createElement(Switch, {checked: true, ariaLabel: 'Enable memory', onCheckedChange() {}}), createElement(Slider, {value: 40, ariaLabel: 'Contrast', onValueChange() {}}), createElement(Visibility), createElement(ContentVisibility, {visible: false}, createElement(ContentVisibility, {visible: true}, createElement(Visibility)))))));
`,
    );
    const html = execFileSync(process.execPath, ["consumer.mjs"], {
      cwd: dir,
      encoding: "utf8",
      env: { ...process.env, NODE_ENV: "production" },
    });
    assert.match(html, /Review/);
    assert.match(html, />\+18</);
    assert.match(html, />−6</);
    assert.match(html, /aria-label="View mode"/);
    assert.match(html, /data-slot="item-highlight"/);
    assert.match(html, /aria-label="Enable memory"/);
    assert.match(html, /aria-label="Contrast"/);
    assert.match(html, /aria-label="Memory"/);
    assert.match(html, /content-visible.*content-hidden/);
    assert.match(html, /width="20"/);
    assert.match(html, /class="x/);
    const css = await readFile(
      path.join(dir, "node_modules/@flame/ui/dist/styles.css"),
      "utf8",
    );
    assert.match(css, /--color-accent/);
    assert.match(css, /:focus-visible/);
  } finally {
    await rm(dir, { recursive: true, force: true });
  }
});
