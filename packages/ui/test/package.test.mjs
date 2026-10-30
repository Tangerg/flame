import assert from "node:assert/strict";
import { test } from "node:test";
import { mkdtemp, writeFile, readFile, rm } from "node:fs/promises";
import { execFileSync } from "node:child_process";
import { tmpdir } from "node:os";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { createConsumerLock } from "../../../tooling/npm-consumer-lock.mjs";

test("packed UI and icons work without the host or a StyleX compiler", async () => {
  const dir = await mkdtemp(path.join(tmpdir(), "flame-ui-consumer-"));
  try {
    const archive = async (url) => {
      const directory = fileURLToPath(url);
      const packed = JSON.parse(
        execFileSync(
          "npm",
          ["pack", "--ignore-scripts", "--json", "--pack-destination", dir],
          { cwd: directory, encoding: "utf8" },
        ),
      );
      const manifest = JSON.parse(
        await readFile(path.join(directory, "package.json")),
      );
      return {
        manifest,
        resolved: `file:${path.join(dir, packed[0].filename)}`,
        integrity: packed[0].integrity,
      };
    };
    const icons = await archive(new URL("../../icons", import.meta.url));
    const ui = await archive(new URL("../", import.meta.url));
    const consumer = {
      private: true,
      type: "module",
      dependencies: {
        "@flame/ui": ui.resolved,
        "@flame/icons": icons.resolved,
        react: ui.manifest.peerDependencies.react,
        "react-dom": ui.manifest.peerDependencies["react-dom"],
      },
    };
    const rootLock = JSON.parse(
      await readFile(new URL("../../../package-lock.json", import.meta.url)),
    );
    await writeFile(path.join(dir, "package.json"), JSON.stringify(consumer));
    await writeFile(
      path.join(dir, "package-lock.json"),
      JSON.stringify(createConsumerLock(rootLock, consumer, [icons, ui])),
    );
    execFileSync(
      "npm",
      ["install", "--offline", "--ignore-scripts", "--no-audit", "--no-fund"],
      { cwd: dir, stdio: "pipe" },
    );
    for (const name of ["@flame/web", "@stylexjs/babel-plugin"]) {
      await assert.rejects(
        readFile(path.join(dir, "node_modules", name, "package.json")),
        { code: "ENOENT" },
      );
    }
    await writeFile(
      path.join(dir, "consumer.mjs"),
      `import { createElement } from 'react';
import { renderToStaticMarkup } from 'react-dom/server';
import { Button, Callout, CalloutLabelsProvider, ContentVisibility, DiffStat, EvidenceImage, FontScaleProvider, IconButton, ImageViewport, Popover, ProgressBar, Segmented, Slider, Switch, TooltipProvider, useContentVisible } from '@flame/ui';
import { Brain } from '@flame/icons/react';
function Visibility() { return createElement('output', null, useContentVisible() ? 'content-visible' : 'content-hidden'); }
console.log(renderToStaticMarkup(createElement(EvidenceImage, {src: 'captured-page.png', alt: 'Captured page'})));
console.log(renderToStaticMarkup(createElement(ImageViewport, {image: {src: 'captured-page.png', alt: 'Captured page'}, label: 'Expanded captured page'})));
console.log(renderToStaticMarkup(createElement(CalloutLabelsProvider, {labels: {note: '宿主说明', tip: 'Host tip', important: 'Host important', warning: 'Host warning', caution: 'Host caution'}}, createElement(Callout, {kind: 'note'}, 'Independent Callout body'))));
console.log(renderToStaticMarkup(createElement(ProgressBar, {value: 3, max: 4, label: 'Review coverage', labelVisibility: 'visible', locale: 'en'})));
console.log(renderToStaticMarkup(createElement(Popover.Root, null, createElement(Popover.Trigger, {render: createElement(Button)}, 'Open details'), createElement(Popover.Content, {'aria-label': 'Portable details'}, 'Captured detail'))));
console.log(renderToStaticMarkup(createElement(FontScaleProvider, {fontSize: 18}, createElement(TooltipProvider, null, createElement(Button, null, 'Review'), createElement(DiffStat, {added: 18, removed: 6}), createElement(IconButton, {icon: Brain, 'aria-label': 'Memory'}), createElement(Segmented, {value: 'list', ariaLabel: 'View mode', options: [{value: 'list', label: 'List'}, {value: 'grid', label: 'Grid'}], onChange() {}}), createElement(Switch, {checked: true, ariaLabel: 'Enable memory', onCheckedChange() {}}), createElement(Slider, {value: 40, ariaLabel: 'Contrast', onValueChange() {}}), createElement(Visibility), createElement(ContentVisibility, {visible: false}, createElement(ContentVisibility, {visible: true}, createElement(Visibility)))))));
`,
    );
    const html = execFileSync(process.execPath, ["consumer.mjs"], {
      cwd: dir,
      encoding: "utf8",
      env: { ...process.env, NODE_ENV: "production" },
    });
    assert.match(html, /Review/);
    assert.match(html, /View image: Captured page/);
    assert.match(html, /Expanded captured page/);
    assert.match(html, /captured-page\.png/);
    assert.match(html, /宿主说明/);
    assert.match(html, /Independent Callout body/);
    assert.match(html, /Open details/);
    assert.match(html, /Review coverage/);
    assert.match(html, /aria-valuenow="3"/);
    assert.match(html, /aria-valuemax="4"/);
    assert.match(html, />75%</);
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
