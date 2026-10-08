import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";
import { test } from "node:test";
import { chromium, webkit } from "@playwright/test";
import { assertCarrierIsolation } from "./plugin-carrier-assertions.mjs";

const fixture = await readFile(
  new URL("../../testdata/plugin-carrier/index.html", import.meta.url),
);

for (const [name, engine] of Object.entries({ chromium, webkit })) {
  test(`${name}: brokered opaque-origin content respects the carrier boundary`, async (t) => {
    const escapedRequests = [];
    const server = createServer((request, response) => {
      if (request.url?.startsWith("/carrier-")) escapedRequests.push(request.url);
      response.writeHead(200, { "Content-Type": "text/html; charset=utf-8" });
      response.end(fixture);
    });
    await new Promise((resolve, reject) => {
      server.once("error", reject);
      server.listen(0, "127.0.0.1", resolve);
    });
    let browser;
    t.after(async () => {
      try {
        await browser?.close();
      } finally {
        await new Promise((resolve, reject) =>
          server.close((error) => (error ? reject(error) : resolve())),
        );
      }
    });
    const address = server.address();
    const origin = `http://127.0.0.1:${address.port}`;
    browser = await engine.launch({ headless: true });
    const context = await browser.newContext();
    await context.addCookies([{ name: "flame_carrier_cookie", value: "fixture", url: origin }]);
    const page = await context.newPage();
    await page.goto(origin);
    assert.ok(
      await page.evaluate(() => document.cookie.includes("flame_carrier_cookie=fixture")),
      "the trusted host must see its cookie positive control",
    );
    await page.waitForFunction(() => window.carrierResult !== undefined);
    const result = await page.evaluate(() => window.carrierResult);
    assertCarrierIsolation(result);
    assert.equal(result.native, false);
    assert.equal(result.hostOrigin, origin);
    assert.equal(result.frames[0].nativeHandler, false);
    assert.deepEqual(escapedRequests, [], "blocked requests must not reach their effect owner");
    assert.equal(page.url(), `${origin}/`, "the plugin must not navigate the trusted host");
  });
}
