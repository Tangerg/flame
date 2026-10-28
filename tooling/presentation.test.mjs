import assert from "node:assert/strict";
import test from "node:test";
import vm from "node:vm";
import { nativePlist, presentationOutputs } from "./presentation.mjs";
import { fileURLToPath } from "node:url";

const repository = fileURLToPath(new URL("../", import.meta.url));

test("native bundle metadata escapes source text and uses actual plist booleans", () => {
  const product = {
    name: "A&B <Client>",
    slug: "client",
    identifier: "org.example.client",
    version: "1.2.3",
    description: 'Read "quoted" & <values>',
    copyright: "Owner's work",
  };
  for (const development of [false, true]) {
    const plist = nativePlist(product, development);
    assert.match(plist, /A&amp;B &lt;Client&gt;/);
    assert.match(plist, /Read &quot;quoted&quot; &amp; &lt;values&gt;/);
    assert.match(plist, /Owner&apos;s work/);
    assert.match(plist, /<key>NSHighResolutionCapable<\/key>\s*<true\/>/);
    assert.equal(plist.includes("NSAllowsLocalNetworking"), development);
    assert.doesNotMatch(plist, /<string>true<\/string>/);
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
