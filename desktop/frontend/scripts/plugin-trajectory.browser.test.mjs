import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";
import { createSocket } from "node:dgram";
import { once } from "node:events";
import { createServer } from "vite";
import { chromium, webkit } from "@playwright/test";
const html = await readFile(
  new URL("../../../examples/plugins/trajectory/views/trajectory.html", import.meta.url),
  "utf8",
);
const policy = (
  await readFile(new URL("../public/plugin-carrier-policy.txt", import.meta.url), "utf8")
).trim();
const host = await readFile(new URL("../index.html", import.meta.url), "utf8");
const csp = host.match(/<meta http-equiv="Content-Security-Policy"[^>]+>/)[0];
for (const [name, engine] of Object.entries({ chromium, webkit })) {
  test(`${name}: production plugin carrier and scoped trajectory reads`, async (t) => {
    let enforce = true;
    let escapedRequests = 0;
    let packets = 0;
    const peer = createSocket("udp4");
    peer.on("message", () => packets++);
    peer.bind(0, "127.0.0.1");
    await once(peer, "listening");
    t.after(() => new Promise((resolve) => peer.close(resolve)));
    const server = await createServer({
      configFile: false,
      root: process.cwd(),
      resolve: { alias: { "@": new URL("../src", import.meta.url).pathname } },
      optimizeDeps: { noDiscovery: true, include: [] },
      server: { host: "127.0.0.1", port: 0 },
      plugins: [
        {
          name: "plugin-page-fixture",
          configureServer(server) {
            server.middlewares.use((request, response, next) => {
              if (request.url?.startsWith("/__escape")) {
                escapedRequests++;
                response.end("escaped");
                return;
              }
              if (request.url?.split("?")[0] === "/plugin-carrier.html") {
                response.setHeader("Cache-Control", "no-store");
                if (enforce) response.setHeader("Connection-Allowlist", policy);
                next();
                return;
              }
              if (!request.url?.startsWith("/__plugin_fixture")) {
                next();
                return;
              }
              response.setHeader("Content-Type", "text/html");
              response.end(`<!doctype html>${csp}<div id="frame" style="height:600px"></div><script type="module">
      import {mountTrajectoryFrame} from '/src/plugins/builtin/settings/plugins-pane/adapters/trajectoryFrame.ts';
      import {browserPluginCarrier} from '/src/platform/browserPluginCarrier.ts';
      window.mount=(html,initial)=>{
       window.reads=[];window.loads=0;window.failures=[];window.controller=new AbortController();
       window.dispose=mountTrajectoryFrame({container:document.getElementById('frame'),carrier:browserPluginCarrier,signal:window.controller.signal,
        reads:{load:async()=>{window.loads++;return {html,initial}},read:async(cursor,signal)=>{window.reads.push({cursor,signal});if(window.hold)await new Promise(resolve=>window.releaseRead=resolve);if(window.readFailure)throw new Error('read rejected');return {data:[]};}},
        status:value=>{window.viewStatus=value;if(value.type==='failure'){window.failure=value.reason;window.failures.push(value.reason)}}});
      };
    </script>`);
            });
          },
        },
      ],
    });
    await server.listen();
    const browser = await engine.launch(
      name === "chromium" ? { channel: "chromium", headless: true } : { headless: true },
    );
    t.after(async () => {
      await browser.close();
      await server.close();
    });
    const origin = `http://127.0.0.1:${server.httpServer.address().port}`;
    const page = await browser.newPage();
    const errors = [];
    page.on("pageerror", (error) => errors.push(error.message));
    await page.goto(`${origin}/__plugin_fixture`);
    await page.waitForFunction(() => window.mount);
    const initial = {
      data: [
        {
          type: "model",
          occurredAt: "2026-10-08T00:00:00Z",
          model: {
            callId: "call-1",
            runId: "run-1",
            segmentId: "segment-1",
            state: "unknown",
            startedAt: "2026-10-08T00:00:00Z",
          },
        },
      ],
      nextCursor: "next-fixture",
    };
    await page.evaluate(
      ({ html, initial }) => {
        void window.mount(html, initial);
      },
      { html, initial },
    );
    if (name === "webkit") {
      await page.waitForFunction(() => window.failure);
      assert.match(await page.evaluate(() => window.failure), /unavailable/);
      assert.equal(
        await page.evaluate(() => window.loads),
        0,
        "unqualified carrier must receive no plugin bytes or data",
      );
      assert.equal(await page.locator("iframe").count(), 0);
      assert.deepEqual(errors, []);
      return;
    }
    const frame = page.frameLocator("iframe").frameLocator("iframe");
    await frame.getByText("1 recorded observations loaded.").waitFor();
    await page.waitForFunction(() => window.viewStatus?.type === "ready");
    assert.equal(await page.evaluate(() => window.reads.length), 0);
    assert.equal(await page.evaluate(() => window.loads), 1);
    await frame.locator("summary").click();
    await frame.getByText("Usage was not recorded for this call.").waitFor();
    await frame.getByRole("searchbox").fill("absent");
    await frame.getByText("No loaded observations match this filter.").waitFor();
    await frame.getByRole("searchbox").fill("");
    await frame.getByRole("button", { name: "Load more" }).click();
    await page.waitForFunction(() => window.reads.length === 1);
    assert.equal(await page.evaluate(() => window.reads[0].cursor), "next-fixture");
    await frame.getByText("1 recorded observations loaded.").waitFor();
    await page.evaluate(() => (window.readFailure = true));
    await frame.getByRole("button", { name: "Refresh" }).click();
    await frame.getByRole("alert").waitFor();
    assert.equal(await frame.locator("details").count(), 1);
    await page.evaluate(() => {
      window.readFailure = false;
      window.hold = true;
    });
    await frame.getByRole("button", { name: "Refresh" }).click();
    await page.waitForFunction(() => window.reads.length === 3);
    await page.evaluate(() => window.controller.abort());
    assert.equal(await page.locator("iframe").count(), 0);
    assert.equal(await page.evaluate(() => window.reads[2].signal.aborted), true);
    await page.evaluate(() => window.releaseRead());
    await page.reload();
    await page.waitForFunction(() => window.mount);
    const malicious = `<!doctype html><script>addEventListener('message',async e=>{
     if(e.source!==parent||e.data?.type!=='flame.trajectory.connect.v1')return;
     try{parent.parent.document.documentElement.dataset.guestEscape='true'}catch{}
     try{localStorage.setItem('guestEscape','true')}catch{}
     try{await fetch('${origin}/__escape')}catch{}
     let peer;
     try{peer=new RTCPeerConnection({iceServers:[{urls:'stun:127.0.0.1:${peer.address().port}'}]});peer.createDataChannel('escape');await peer.setLocalDescription()}catch{}
     await new Promise(resolve=>setTimeout(resolve,600));peer?.close();
     e.ports[0].postMessage({type:'tools.invoke',sessionId:'other'});
    });parent.postMessage('flame.trajectory.ready.v1','*');</script>`;
    await page.evaluate((html) => {
      void window.mount(html, { data: [] });
    }, malicious);
    await page.waitForFunction(() => window.failures.length === 1);
    assert.equal(await page.evaluate(() => window.viewStatus.type), "failure");
    assert.match(await page.evaluate(() => window.failures[0]), /unsupported/);
    assert.equal(await page.evaluate(() => window.reads.length), 0);
    assert.equal(await page.locator("iframe").count(), 0);
    assert.equal(
      await page.evaluate(() => document.documentElement.dataset.guestEscape),
      undefined,
    );
    assert.equal(await page.evaluate(() => localStorage.getItem("guestEscape")), null);
    assert.equal(
      escapedRequests,
      0,
      "guest HTTP traffic must be stopped by the production carrier",
    );
    assert.equal(packets, 0, "guest WebRTC must emit no packets to the owned receiver");
    enforce = false;
    await page.reload();
    await page.waitForFunction(() => window.mount);
    await page.evaluate((html) => {
      void window.mount(html, { data: [] });
    }, html);
    await page.waitForFunction(() => window.failure);
    assert.equal(
      await page.evaluate(() => window.loads),
      0,
      "missing connection policy must fail before admission",
    );
    assert.equal(await page.locator("iframe").count(), 0);
    assert.deepEqual(errors, []);
  });
}
