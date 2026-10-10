import assert from "node:assert/strict";
import { readFile } from "node:fs/promises";
import { test } from "node:test";
import { createSocket } from "node:dgram";
import { once } from "node:events";
import { createServer } from "vite";
import { chromium, webkit } from "@playwright/test";
const html = await readFile(
  new URL("../../../plugins/trajectory/views/trajectory.html", import.meta.url),
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
      window.mount=(html,initial,scheme="light")=>{
       window.reads=[];window.loads=0;window.failures=[];window.controller=new AbortController();
       window.dispose=mountTrajectoryFrame({container:document.getElementById('frame'),carrier:browserPluginCarrier,signal:window.controller.signal,scheme,
        reads:{load:async()=>{window.loads++;return {html,initial}},read:async(cursor,signal)=>{window.reads.push({cursor,signal});if(window.hold)await new Promise(resolve=>window.releaseRead=resolve);if(window.readFailure)throw new Error('read rejected');return cursor ? {data:[{type:"model",occurredAt:"2026-10-07T00:00:00Z",model:{callId:"older-call",runId:"run-2",segmentId:"segment-2",state:"failed",startedAt:"2026-10-07T00:00:00Z"}}]} : initial;}},
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
            settledAt: "2026-10-08T00:01:00Z",
            startedAt: "2026-10-08T00:00:00Z",
          },
        },
        {
          type: "model",
          occurredAt: "2026-10-08T00:00:00Z",
          model: {
            callId: "zero-call",
            runId: "run-2",
            segmentId: "segment-2",
            state: "completed",
            startedAt: "2026-10-08T00:00:00Z",
            settledAt: "2026-10-08T00:00:00Z",
            firstOutputLatencyMillis: 0,
            usage: { inputTokens: 0, outputTokens: 0, cacheReadTokens: 0 },
          },
        },
        {
          type: "item",
          occurredAt: "2026-10-08T00:00:00Z",
          item: {
            id: "tool-1",
            type: "toolCall",
            runId: "run-1",
            status: "completed",
            durationMillis: 0,
            tool: { name: "shell" },
            arguments: { command: "go test ./..." },
          },
        },
        {
          type: "item",
          occurredAt: "2026-10-08T00:00:00Z",
          item: {
            id: "reasoning-1",
            type: "reasoning",
            runId: "run-1",
            status: "completed",
            redacted: true,
            text: "must-never-render",
          },
        },
        {
          type: "item",
          occurredAt: "2026-10-08T00:00:00Z",
          item: {
            id: "image-1",
            type: "agentMessage",
            runId: "run-1",
            status: "completed",
            content: [
              {
                type: "image",
                mime: "image/png",
                data: "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADElEQVR4nGOYuOs/AAQqAktocgm/AAAAAElFTkSuQmCC",
              },
            ],
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
    await frame.getByText("5 recorded observations loaded on this page.").waitFor();
    assert.equal(
      await frame.locator("html").evaluate((element) => element.style.colorScheme),
      "light",
    );
    await page.waitForFunction(() => window.viewStatus?.type === "ready");
    assert.equal(await page.evaluate(() => window.reads.length), 0);
    assert.equal(await page.evaluate(() => window.loads), 1);
    assert.equal(await frame.locator("img").count(), 0);
    const unknown = frame.locator('[data-trajectory-record="model:call-1"]');
    await unknown.locator("summary").click();
    await unknown.getByText("Usage was not recorded for this call.").waitFor();
    assert.equal(await unknown.getByText("Wall duration (ms)", { exact: true }).count(), 0);
    const zeros = frame.locator('[data-trajectory-record="model:zero-call"]');
    await zeros.locator("summary").click();
    await zeros.getByText("Input tokens", { exact: true }).waitFor();
    assert.deepEqual(await zeros.locator("dd").allTextContents(), [
      "run-2",
      "segment-2",
      "2026-10-08T00:00:00Z",
      "0",
      "0",
      "0",
      "0",
      "0",
    ]);
    assert.equal(await zeros.getByText("Cache write tokens", { exact: true }).count(), 0);
    const tool = frame.locator('[data-trajectory-record="item:tool-1"]');
    await tool.locator("summary").click();
    await tool.getByText("Execution duration (ms)", { exact: true }).waitFor();
    assert.equal(await tool.locator("dd").last().textContent(), "0");
    const reasoning = frame.locator('[data-trajectory-record="item:reasoning-1"]');
    await reasoning.locator("summary").click();
    await reasoning.getByText("Reasoning was redacted.").waitFor();
    assert.doesNotMatch(await reasoning.textContent(), /must-never-render/);
    await frame.getByRole("searchbox").fill("must-never-render");
    await frame.getByText("No records on this page match this filter.").waitFor();
    await frame.getByRole("searchbox").fill("");
    await frame.getByRole("combobox", { name: "Run filter" }).selectOption("run-2");
    assert.equal(await frame.locator("details").count(), 1);
    await frame.getByRole("combobox", { name: "Run filter" }).selectOption("");
    await frame.getByRole("combobox", { name: "Observation type" }).selectOption("attention");
    assert.equal(await frame.locator("details").count(), 1);
    await frame.getByRole("combobox", { name: "Observation type" }).selectOption("all");
    await frame.locator('[data-trajectory-record="item:image-1"] summary').click();
    await frame.locator("img").waitFor();
    await frame.locator("img").evaluate((element) => {
      if (element.complete) return;
      return new Promise((resolve) => (element.onload = element.onerror = resolve));
    });
    assert.equal(await frame.locator("img").evaluate((element) => element.naturalWidth), 1);
    assert.equal(await frame.getByRole("button").count(), 3);
    await frame.getByRole("button", { name: "Older records" }).click();
    await page.waitForFunction(() => window.reads.length === 1);
    assert.equal(await page.evaluate(() => window.reads[0].cursor), "next-fixture");
    await frame.getByText("1 recorded observations loaded on this page.").waitFor();
    assert.equal(await frame.locator("details").count(), 1);
    assert.equal(await frame.locator('[data-trajectory-record="model:call-1"]').count(), 0);
    await page.evaluate(() => (window.readFailure = true));
    await frame.getByRole("button", { name: "Newer records" }).click();
    await frame.getByRole("alert").waitFor();
    assert.equal(await frame.locator('[data-trajectory-record="model:older-call"]').count(), 1);
    assert.equal(await frame.getByRole("button", { name: "Newer records" }).isEnabled(), true);
    await page.evaluate(() => (window.readFailure = false));
    await frame.getByRole("button", { name: "Newer records" }).click();
    await frame.getByText("5 recorded observations loaded on this page.").waitFor();
    assert.equal(await page.evaluate(() => window.reads[2].cursor), undefined);
    assert.equal(await frame.getByRole("button", { name: "Newer records" }).isEnabled(), false);
    await page.evaluate(() => (window.hold = true));
    await frame.getByRole("button", { name: "Refresh" }).click();
    await page.waitForFunction(() => window.reads.length === 4);
    await page.evaluate(() => window.controller.abort());
    assert.equal(await page.locator("iframe").count(), 0);
    assert.equal(await page.evaluate(() => window.reads[3].signal.aborted), true);
    await page.evaluate(() => window.releaseRead());
    await page.reload();
    await page.waitForFunction(() => window.mount);
    await page.evaluate(({ html, initial }) => void window.mount(html, initial, "dark"), {
      html,
      initial,
    });
    await frame.getByText("5 recorded observations loaded on this page.").waitFor();
    assert.equal(
      await frame.locator("html").evaluate((element) => element.style.colorScheme),
      "dark",
    );
    await page.evaluate(() => window.dispose());
    await page.reload();
    await page.waitForFunction(() => window.mount);
    const malicious = `<!doctype html><script>addEventListener('message',async e=>{
     if(e.source!==parent||e.data?.type!=='flame.trajectory.connect.v1')return;
     try{parent.parent.document.documentElement.dataset.guestEscape='true'}catch{}
     try{localStorage.setItem('guestEscape','true')}catch{}
     new Image().src='${origin}/__escape';
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
