import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { createServer } from "node:http";
import { createSocket } from "node:dgram";
import { once } from "node:events";
import { test } from "node:test";
import { chromium, webkit } from "@playwright/test";
import { assertCarrierIsolation } from "./plugin-carrier-assertions.mjs";

const enforcement = { kind: "connection-allowlist" };

const fixture = await readFile(
  new URL("../../testdata/plugin-carrier/index.html", import.meta.url),
  "utf8",
);
const frameEntry = await readFile(
  new URL("../../testdata/plugin-carrier/frame.html", import.meta.url),
);
const networkPolicy = (
  await readFile(
    new URL("../../testdata/plugin-carrier/network-policy.txt", import.meta.url),
    "utf8",
  )
).trim();

async function runProbe(t, context, target, initialize, policy = "enforce") {
  const { origin, iceEndpoint, packets } = target;
  const before = packets();
  const url = new URL(origin);
  url.searchParams.set("carrier-stun", iceEndpoint);
  url.searchParams.set("carrier-policy", policy);
  const page = await context.newPage();
  t.after(() => page.close());
  await page.addInitScript(() => {
    if (window !== window.top) return;
    window.carrierUnhandledRejections = [];
    addEventListener("unhandledrejection", (event) =>
      window.carrierUnhandledRejections.push(String(event.reason)),
    );
  });
  if (initialize) await page.addInitScript(initialize, { origin });
  await page.goto(url.href);
  assert.ok(
    await page.evaluate(() => document.cookie.includes("flame_carrier_cookie=fixture")),
    "the trusted host must see its cookie positive control",
  );
  await page.waitForFunction(() => window.carrierResult !== undefined);
  const { result, encoded, unhandledRejections } = await page.evaluate(() => ({
    result: window.carrierResult,
    encoded: document.getElementById("result").textContent,
    unhandledRejections: window.carrierUnhandledRejections,
  }));
  assert.equal(page.url(), url.href, "the plugin must not navigate the trusted host");
  assert.equal(page.frames().length, 1, "terminal completion must dispose every child frame");
  assert.deepEqual(unhandledRejections, [], "terminal completion must settle every owned wait");
  assert.deepEqual(result, JSON.parse(encoded), "terminal consumers must share one JSON result");
  return { carrier: result, peerPackets: packets() - before };
}

for (const [name, engine] of Object.entries({ chromium, webkit })) {
  test(`${name}: brokered opaque-origin content respects the carrier boundary`, async (t) => {
    const escapedRequests = [];
    const peer = createSocket("udp4");
    let packets = 0;
    peer.on("message", (packet) => {
      if (packet.toString() !== "carrier-peer-control") packets++;
    });
    t.after(() => peer.close());
    const listening = once(peer, "listening", { signal: AbortSignal.timeout(5_000) });
    peer.bind(0, "127.0.0.1");
    await listening;
    const control = once(peer, "message", { signal: AbortSignal.timeout(5_000) });
    peer.send(Buffer.from("carrier-peer-control"), peer.address().port, "127.0.0.1");
    assert.equal((await control)[0].toString(), "carrier-peer-control");
    const server = createServer((request, response) => {
      if (request.url?.startsWith("/carrier-")) escapedRequests.push(request.url);
      response.setHeader("Content-Type", "text/html; charset=utf-8");
      const requested = new URL(request.url, `http://127.0.0.1:${server.address().port}`);
      if (requested.pathname === "/frame.html") {
        const policy = requested.searchParams.get("carrier-policy");
        if (policy === "enforce") response.setHeader("Connection-Allowlist", networkPolicy);
        if (policy === "report")
          response.setHeader("Connection-Allowlist-Report-Only", networkPolicy);
        response.end(frameEntry);
      } else {
        response.end(
          fixture.replaceAll(
            "__CARRIER_FRAME_SOURCE__",
            `http://127.0.0.1:${server.address().port}/frame.html`,
          ),
        );
      }
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
    const target = {
      origin,
      iceEndpoint: `stun:127.0.0.1:${peer.address().port}`,
      packets: () => packets,
    };
    browser = await engine.launch(
      name === "chromium" ? { channel: "chromium", headless: true } : { headless: true },
    );
    const context = await browser.newContext();
    await context.addCookies([{ name: "flame_carrier_cookie", value: "fixture", url: origin }]);
    await t.test("isolates the bound frame", async (t) => {
      const { carrier: result, peerPackets } = await runProbe(t, context, target);
      t.diagnostic(JSON.stringify({ peer: result.frame?.observations.peer, peerPackets }));
      assertCarrierIsolation(result, peerPackets, enforcement);
      assert.equal(result.wails, false);
      assert.equal(result.hostOrigin, origin);
      assert.equal(result.frame.observations.nativeHandler, false);
    });
    await t.test("rejects report-only network policy", async (t) => {
      const { carrier: result, peerPackets } = await runProbe(
        t,
        context,
        target,
        undefined,
        "report",
      );
      assert.ok(peerPackets > 0, "the report-only control must reach the UDP receiver");
      assert.equal(result.frame.observations.peer.policyEnforced, false);
      assert.throws(
        () => assertCarrierIsolation(result, peerPackets, enforcement),
        /a plugin WebRTC packet/,
      );
    });
    await t.test("rejects an absent network policy", async (t) => {
      const { carrier: result, peerPackets } = await runProbe(
        t,
        context,
        target,
        undefined,
        "absent",
      );
      assert.ok(peerPackets > 0, "the unprotected control must reach the UDP receiver");
      assert.equal(result.frame.observations.peer.policyEnforced, false);
      assert.throws(
        () => assertCarrierIsolation(result, peerPackets, enforcement),
        /a plugin WebRTC packet/,
      );
    });
    await t.test("refuses fabricated browser reports", async (t) => {
      const { carrier: result, peerPackets } = await runProbe(t, context, target, () => {
        if (window === window.top) return;
        window.ReportingObserver = class {
          constructor(receive) {
            this.receive = receive;
          }
          observe() {
            this.receive([
              {
                toJSON: () => ({
                  type: "connection-allowlist",
                  body: { connection: "webrtc", disposition: "enforce" },
                }),
              },
            ]);
          }
          disconnect() {}
        };
      });
      assert.match(result.error, /toJSON|undefined/);
      assert.throws(
        () => assertCarrierIsolation(result, peerPackets, enforcement),
        /carrier probe failed/,
      );
    });
    await t.test(
      "refuses JavaScript withdrawal of the WebRTC API as policy evidence",
      async (t) => {
        const { carrier: result, peerPackets } = await runProbe(t, context, target, () => {
          if (window !== window.top) window.RTCPeerConnection = undefined;
        });
        assert.equal(peerPackets, 0);
        assert.equal(result.frame.observations.peer.available, false);
        assert.throws(
          () => assertCarrierIsolation(result, peerPackets, enforcement),
          /the WebRTC positive control must exist/,
        );
      },
    );
    await t.test("rejects an unclosed port", async (t) => {
      const { carrier: result, peerPackets } = await runProbe(t, context, target, () => {
        if (window === window.top) MessagePort.prototype.close = () => {};
      });
      assert.throws(
        () => assertCarrierIsolation(result, peerPackets, enforcement),
        /a retired channel must stop publication/,
        "the gate must detect an unclosed port before removing its frame",
      );
    });
    await t.test("disposes a failed child", async (t) => {
      const { carrier: result } = await runProbe(t, context, target, () => {
        if (window !== window.top)
          parent.postMessage({ type: "carrier-error", error: "injected frame failure" }, "*");
      });
      assert.match(result.error, /injected frame failure/);
    });
    await t.test("publishes failure when frame evidence cannot be encoded", async (t) => {
      const { carrier: result } = await runProbe(t, context, target, () => {
        if (window === window.top) return;
        addEventListener("securitypolicyviolation", (event) => {
          if (event.effectiveDirective !== "connect-src") return;
          const observations = {};
          observations.loop = observations;
          parent.postMessage({ type: "carrier-child", observations }, "*");
        });
      });
      assert.match(result.error, /circular|cyclic/i);
    });
    await t.test("projects browser evidence through the terminal JSON contract", async (t) => {
      const { carrier: result } = await runProbe(t, context, target, () => {
        if (window === window.top) return;
        addEventListener("securitypolicyviolation", (event) => {
          if (event.effectiveDirective === "connect-src")
            parent.postMessage(
              { type: "carrier-child", observations: { omitted: undefined } },
              "*",
            );
        });
      });
      assert.deepEqual(result.frame.observations, {});
    });
    await t.test("settles a wait when port start fails", async (t) => {
      const { carrier: result } = await runProbe(t, context, target, () => {
        if (window === window.top)
          MessagePort.prototype.start = () => {
            throw new Error("injected port start failure");
          };
      });
      assert.match(result.error, /injected port start failure/);
    });
    await t.test("settles a wait when port send fails", async (t) => {
      const { carrier: result } = await runProbe(t, context, target, () => {
        if (window !== window.top) return;
        const postMessage = MessagePort.prototype.postMessage;
        MessagePort.prototype.postMessage = function (...args) {
          if (args[0] === "ping") throw new Error("injected port send failure");
          return postMessage.apply(this, args);
        };
      });
      assert.match(result.error, /injected port send failure/);
    });
    await t.test("rejects an unexpected fetch failure", async (t) => {
      const { carrier: result } = await runProbe(t, context, target, () => {
        if (window !== window.top)
          window.fetch = () => Promise.reject(new Error("injected fetch probe failure"));
      });
      assert.match(result.error, /injected fetch probe failure/);
    });
    await t.test("requires a trusted CSP witness for a rejected fetch", async (t) => {
      const { carrier: result } = await runProbe(t, context, target, () => {
        if (window === window.top) return;
        window.fetch = () => {
          dispatchEvent(
            new SecurityPolicyViolationEvent("securitypolicyviolation", {
              effectiveDirective: "connect-src",
            }),
          );
          return Promise.reject(new TypeError("injected network failure"));
        };
      });
      assert.match(result.error, /carrier probe timed out/);
    });
    await t.test("requires CSP evidence for the attempted network resource", async (t) => {
      const { carrier: result } = await runProbe(t, context, target, () => {
        if (window === window.top) return;
        const fetch = window.fetch;
        window.fetch = (url) => fetch(`${url}-unrelated`);
      });
      assert.match(result.error, /carrier probe timed out/);
    });
    await t.test("requires CSP evidence for the attempted navigation resource", async (t) => {
      const { carrier: result } = await runProbe(t, context, target, ({ origin }) => {
        if (window === window.top) return;
        addEventListener("message", (event) => {
          if (event.data?.type !== "navigate") return;
          event.stopImmediatePropagation();
          parent.postMessage({ type: "carrier-navigation" }, "*");
          location.href = new URL("/carrier-navigation-unrelated", origin).href;
        });
      });
      assert.match(result.error, /carrier probe timed out/);
    });
    await t.test("rejects a readable network response", async (t) => {
      const { carrier: result, peerPackets } = await runProbe(t, context, target, () => {
        if (window !== window.top) window.fetch = () => Promise.resolve(new Response());
      });
      assert.throws(
        () => assertCarrierIsolation(result, peerPackets, enforcement),
        /plugin frame escaped through network/,
      );
    });
    assert.deepEqual(escapedRequests, [], "blocked requests must not reach their effect owner");
  });
}
