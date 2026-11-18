import assert from "node:assert/strict";
import test from "node:test";
import { nativePlist } from "./presentation.mjs";

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
