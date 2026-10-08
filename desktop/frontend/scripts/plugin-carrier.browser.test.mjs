import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";
import { test } from "node:test";
import { chromium, webkit } from "@playwright/test";
import { assertCarrierIsolation } from "./plugin-carrier-assertions.mjs";

const fixture = await readFile(
  new URL("../../testdata/plugin-carrier/index.html", import.meta.url),
);

async function runProbe(t, context, origin, initialize) {
  const page = await context.newPage();
  t.after(() => page.close());
  await page.addInitScript(() => {
    if (window !== window.top) return;
    window.carrierUnhandledRejections = [];
    addEventListener("unhandledrejection", (event) =>
      window.carrierUnhandledRejections.push(String(event.reason)),
    );
  });
  if (initialize) await page.addInitScript(initialize);
  await page.goto(origin);
  assert.ok(
    await page.evaluate(() => document.cookie.includes("flame_carrier_cookie=fixture")),
    "the trusted host must see its cookie positive control",
  );
  await page.waitForFunction(() => window.carrierResult !== undefined);
  const { result, unhandledRejections } = await page.evaluate(() => ({
    result: window.carrierResult,
    unhandledRejections: window.carrierUnhandledRejections,
  }));
  assert.equal(page.url(), `${origin}/`, "the plugin must not navigate the trusted host");
  assert.equal(page.frames().length, 1, "terminal completion must dispose every child frame");
  assert.deepEqual(unhandledRejections, [], "terminal completion must settle every owned wait");
  return result;
}

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
    await t.test("isolates the bound frame", async (t) => {
      const result = await runProbe(t, context, origin);
      assertCarrierIsolation(result);
      assert.equal(result.native, false);
      assert.equal(result.hostOrigin, origin);
      assert.equal(result.frame.observations.nativeHandler, false);
    });
    await t.test("rejects an unclosed port", async (t) => {
      const result = await runProbe(t, context, origin, () => {
        if (window === window.top) MessagePort.prototype.close = () => {};
      });
      assert.throws(
        () => assertCarrierIsolation(result),
        /a retired channel must stop publication/,
        "the gate must detect an unclosed port before removing its frame",
      );
    });
    await t.test("disposes a failed child", async (t) => {
      const result = await runProbe(t, context, origin, () => {
        if (window !== window.top)
          parent.postMessage({ type: "carrier-error", error: "injected frame failure" }, "*");
      });
      assert.match(result.error, /injected frame failure/);
    });
    await t.test("settles a wait when port start fails", async (t) => {
      const result = await runProbe(t, context, origin, () => {
        if (window === window.top)
          MessagePort.prototype.start = () => {
            throw new Error("injected port start failure");
          };
      });
      assert.match(result.error, /injected port start failure/);
    });
    await t.test("settles a wait when port send fails", async (t) => {
      const result = await runProbe(t, context, origin, () => {
        if (window !== window.top) return;
        const postMessage = MessagePort.prototype.postMessage;
        MessagePort.prototype.postMessage = function (...args) {
          if (args[0] === "ping") throw new Error("injected port send failure");
          return postMessage.apply(this, args);
        };
      });
      assert.match(result.error, /injected port send failure/);
    });
    assert.deepEqual(escapedRequests, [], "blocked requests must not reach their effect owner");
  });
}
