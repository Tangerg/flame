import assert from "node:assert/strict";
import test from "node:test";
import vm from "node:vm";
import { nativePlist, presentationOutputs } from "./presentation.mjs";
import { fileURLToPath } from "node:url";
import { readFile } from "node:fs/promises";
import { transformSync } from "rolldown/utils";

const repository = fileURLToPath(new URL("../", import.meta.url));

test("startup diagnostics project the current catalogs without acquiring runtime translations", async () => {
  const output = await presentationOutputs(repository);
  const source = output.get("apps/web/src/app/startupCopy.generated.ts");
  const compiled = transformSync("startupCopy.generated.ts", source, {
    target: "safari16.1",
  });
  assert.deepEqual(compiled.errors, []);
  const { startupCopy } = await import(
    `data:text/javascript;base64,${Buffer.from(compiled.code).toString("base64")}`
  );
  const { en } = await import(
    new URL("../apps/web/src/lib/i18n/locales/en.ts", import.meta.url)
  );
  for (const [tag, dictionary] of Object.entries(startupCopy)) {
    const catalog =
      tag === "en"
        ? en
        : JSON.parse(
            await readFile(
              new URL(
                `../plugins/locale-${tag.toLowerCase()}/package/io.github.tangerg.flame/messages.json`,
                import.meta.url,
              ),
              "utf8",
            ),
          );
    for (const [key, value] of Object.entries(dictionary))
      assert.equal(value, catalog[key]);
  }
  assert.deepEqual(
    Object.keys(startupCopy).sort(),
    ["de", "en", "es", "fr", "ja", "ko", "zh", "zh-TW"].sort(),
  );
});

test("native bundle metadata escapes source text and uses actual plist booleans", () => {
  const product = {
    name: "A&B <Client>",
    slug: "client",
    identifier: "org.example.client",
    version: "1.2.3",
    description: 'Read "quoted" & <values>',
    copyright: "Owner's work",
    macOS: { minimumVersion: "14.0" },
  };
  for (const development of [false, true]) {
    const plist = nativePlist(product, development);
    assert.match(plist, /A&amp;B &lt;Client&gt;/);
    assert.match(plist, /Read &quot;quoted&quot; &amp; &lt;values&gt;/);
    assert.match(plist, /Owner&apos;s work/);
    assert.match(plist, /<key>NSHighResolutionCapable<\/key>\s*<true\/>/);
    assert.equal(plist.includes("NSAllowsLocalNetworking"), development);
    assert.doesNotMatch(plist, /<string>true<\/string>/);
    assert.match(
      plist,
      /<key>LSMinimumSystemVersion<\/key>\s*<string>14\.0<\/string>/,
    );
  }
});

test("prepaint honors a fixed scheme without requiring a paint cache", async () => {
  const output = await presentationOutputs(repository);
  const script = output.get("apps/web/public/appearance.generated.js");
  for (const theme of ["light", "dark"]) {
    const classes = [];
    const documentElement = {
      dataset: {},
      style: {},
      classList: { add: (value) => classes.push(value) },
    };
    vm.runInNewContext(script, {
      document: { documentElement },
      window: { matchMedia: () => ({ matches: theme === "light" }) },
      localStorage: {
        getItem: (key) =>
          key.endsWith("appearance")
            ? JSON.stringify({ state: { theme } })
            : null,
      },
    });
    assert.deepEqual(classes, [`theme-${theme}`]);
    assert.match(documentElement.style.backgroundColor, /^#[\da-f]{6}$/i);
  }
});
