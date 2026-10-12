import assert from "node:assert/strict";
import { readFile, writeFile, mkdtemp } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { test } from "node:test";
import { fileURLToPath } from "node:url";
import { createSocket } from "node:dgram";
import { once } from "node:events";
import { createServer } from "vite";
import { chromium, webkit, expect } from "@playwright/test";
const reviewDirectory = await mkdtemp(join(tmpdir(), "flame-page-review-"));
console.log("Review snapshots:", reviewDirectory);
const axeSource = await readFile(fileURLToPath(import.meta.resolve("axe-core/axe.min.js")), "utf8");
const uiStyles = `/@fs${fileURLToPath(import.meta.resolve("@flame/ui/styles.css"))}`;
const themeModule = `/@fs${fileURLToPath(import.meta.resolve("@flame/ui/theme"))}`;
async function choose(frame, label, option) {
  await frame.getByRole("combobox", { name: label }).click();
  await frame.getByRole("option", { name: option, exact: true }).click();
}
const html = await readFile(
  new URL("../../../plugins/trajectory/views/trajectory.html", import.meta.url),
  "utf8",
);
const memoryHTML = await readFile(
  new URL("../../../plugins/memory/views/memory.html", import.meta.url),
  "utf8",
);
const scheduleHTML = await readFile(
  new URL("../../../plugins/schedules/views/schedules.html", import.meta.url),
  "utf8",
);
const usageHTML = await readFile(
  new URL("../../../plugins/usage/views/usage.html", import.meta.url),
  "utf8",
);
const skillsHTML = await readFile(
  new URL("../../../plugins/skills/views/skills.html", import.meta.url),
  "utf8",
);
const policy = (
  await readFile(new URL("../public/plugin-carrier-policy.txt", import.meta.url), "utf8")
).trim();
const host = await readFile(new URL("../index.html", import.meta.url), "utf8");
const csp = host.match(/<meta http-equiv="Content-Security-Policy"[^>]+>/)[0];
for (const [name, engine] of Object.entries({ chromium, webkit })) {
  test(`${name}: production plugin carrier and scoped trajectory, memory, schedule, usage, Skill, Diff, recorded result, captured file, current-client diagnostics and static content reads`, async (t) => {
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
      resolve: {
        dedupe: ["react", "react-dom", "@base-ui/react"],
        alias: { "@": new URL("../src", import.meta.url).pathname },
      },
      optimizeDeps: { noDiscovery: true, include: ["react", "react/jsx-runtime"] },
      server: {
        host: "127.0.0.1",
        port: 0,
        fs: { allow: [process.cwd(), new URL("../../../ui", import.meta.url).pathname] },
      },
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
              response.end(`<!doctype html>${csp}<link rel="stylesheet" href="${uiStyles}"><link rel="stylesheet" href="/src/styles/globals.css"><style>html,body{min-width:0}</style><div id="frame" style="height:600px"></div><script type="module">
      import {openPluginViewFrame} from '/src/plugins/builtin/settings/plugins-pane/adapters/pluginViewFrame.ts';
      import {captureTheme} from '${themeModule}'; window.captureTheme=captureTheme;
      import {uiTypeLadderCssVariables} from '/src/plugins/builtin/theme/kit/typeLadder.ts';
      import {productIconScaleCssVariables} from '/src/lib/productIconScale.ts';
      window.projectFont=font=>Object.entries({...uiTypeLadderCssVariables(font),...productIconScaleCssVariables(font)}).forEach(([key,value])=>document.documentElement.style.setProperty(key,value));
      import {browserPluginCarrier} from '/src/platform/browserPluginCarrier.ts';
      window.mount=(html,initial,scheme="light")=>{
       window.reads=[];window.loads=0;window.failures=[];window.controller=new AbortController();
       window.frameController=openPluginViewFrame({container:document.getElementById('frame'),carrier:browserPluginCarrier,signal:window.controller.signal,theme:captureTheme(document.documentElement,scheme,14),
        reads:{binding:{installationId:"fixture",digest:"fixture",contributionId:"fixture"},resource:async()=>{window.loads++;return html},initial:async()=>initial,read:async(cursor,signal)=>{window.reads.push({cursor,signal});if(window.hold)await new Promise(resolve=>window.releaseRead=resolve);if(window.readFailure)throw new Error('read rejected');return cursor ? window.continuation ?? {data:[{type:"model",occurredAt:"2026-10-07T00:00:00Z",model:{callId:"older-call",runId:"run-2",segmentId:"segment-2",state:"failed",startedAt:"2026-10-07T00:00:00Z"}}]} : initial;}},
        status:value=>{window.viewStatus=value;if(value.type==='failure'){window.failure=value.reason;window.failures.push(value.reason)}}});
       window.dispose=window.frameController.close;
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
    await page
      .waitForFunction(() => window.mount)
      .catch((cause) => {
        throw new AggregateError(
          errors.map((message) => new Error(message)),
          "production plugin fixture failed to initialize",
          { cause },
        );
      });
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
      assert.equal(await page.evaluate(() => window.viewStatus.stage), "carrier");
      assert.equal(
        await page.evaluate(() => window.failure),
        "The plugin carrier could not be opened.",
      );
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
    async function reviewAppearance(label) {
      const before = await page.evaluate(() => ({
        loads: window.loads,
        reads: window.reads.length,
      }));
      await frame.locator("html").evaluate((root, source) => {
        const script = root.ownerDocument.createElement("script");
        script.textContent = source;
        root.ownerDocument.head.append(script);
      }, axeSource);
      for (const scheme of ["light", "dark"]) {
        await page.setViewportSize({ width: 320, height: 720 });
        await page.evaluate((scheme) => {
          const root = document.documentElement;
          root.classList.toggle("theme-dark", scheme === "dark");
          window.projectFont(18);
          window.frameController.updateTheme(window.captureTheme(root, scheme, 18));
        }, scheme);
        await expect(frame.locator("html")).toHaveAttribute("data-flame-ui-font-size", "18");
        const extent = await frame.locator("html").evaluate((root) => ({
          width: root.clientWidth,
          scroll: root.scrollWidth,
          boxes: [...root.querySelectorAll("*")]
            .filter(
              (el) =>
                el.scrollWidth > el.clientWidth && getComputedStyle(el).overflowX === "visible",
            )
            .slice(0, 12)
            .map((el) => ({
              tag: el.tagName,
              id: el.id,
              width: el.clientWidth,
              scroll: el.scrollWidth,
              display: getComputedStyle(el).display,
              text: el.textContent.slice(0, 60),
            })),
        }));
        if (extent.scroll > extent.width) console.log(label, JSON.stringify(extent));
        const preview = await frame.locator("html").evaluate((root) => {
          const copy = root.cloneNode(true);
          copy.querySelectorAll("script").forEach((script) => script.remove());
          return "<!doctype html>" + copy.outerHTML;
        });
        await writeFile(join(reviewDirectory, `${label}-${scheme}.html`), preview);
        assert.equal(extent.scroll <= extent.width, true, `${label} must reflow at 320px`);
        const failures = await frame.locator("html").evaluate(async (root) => {
          const result = await root.ownerDocument.defaultView.axe.run(root.ownerDocument, {
            runOnly: { type: "tag", values: ["wcag2a", "wcag2aa", "wcag21aa", "wcag22aa"] },
          });
          return result.violations.map((issue) => ({
            id: issue.id,
            nodes: issue.nodes.map((node) => ({
              target: node.target,
              reason: node.failureSummary,
            })),
          }));
        });
        assert.deepEqual(failures, [], `${label} ${scheme} accessibility`);
        await frame
          .locator("html")
          .evaluate((root) => root.ownerDocument.defaultView.scrollTo(0, 0));
        await page.screenshot({ path: `/tmp/flame-${label}-${scheme}.png` });
      }
      await frame.locator("html").evaluate((root) => (root.dir = "rtl"));
      assert.equal(
        await frame.locator("html").evaluate((root) => root.scrollWidth <= root.clientWidth),
        true,
        `${label} RTL reflow`,
      );
      await frame.locator("html").evaluate((root) => (root.dir = "ltr"));
      await page.setViewportSize({ width: 640, height: 720 });
      await frame.locator("html").evaluate((root) => (root.style.zoom = "2"));
      assert.equal(
        await frame.locator("html").evaluate((root) => root.scrollWidth <= root.clientWidth),
        true,
        `${label} 200% zoom`,
      );
      await frame.locator("html").evaluate((root) => (root.style.zoom = ""));
      assert.deepEqual(
        await page.evaluate(() => ({ loads: window.loads, reads: window.reads.length })),
        before,
        "appearance cannot reload resources or business facts",
      );
    }

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
    await unknown.getByRole("button").first().click();
    await unknown.getByText("Usage was not recorded for this call.").waitFor();
    await reviewAppearance("trajectory");
    assert.equal(await unknown.getByText("Wall duration (ms)", { exact: true }).count(), 0);
    const zeros = frame.locator('[data-trajectory-record="model:zero-call"]');
    await zeros.getByRole("button").first().click();
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
    await tool.getByRole("button").first().click();
    await tool.getByText("Execution duration (ms)", { exact: true }).waitFor();
    assert.equal(await tool.locator("dd").last().textContent(), "0");
    const reasoning = frame.locator('[data-trajectory-record="item:reasoning-1"]');
    await reasoning.getByRole("button").first().click();
    await reasoning.getByText("Reasoning was redacted.").waitFor();
    assert.doesNotMatch(await reasoning.textContent(), /must-never-render/);
    await frame.getByRole("searchbox").fill("must-never-render");
    await frame.getByText("No records on this page match this filter.").waitFor();
    await frame.getByRole("searchbox").fill("");
    await choose(frame, "Run filter", "run-2");
    assert.equal(await frame.locator("[data-trajectory-record]").count(), 1);
    await choose(frame, "Run filter", "All Runs");
    await choose(frame, "Observation type", "Needs attention");
    assert.equal(await frame.locator("[data-trajectory-record]").count(), 1);
    await choose(frame, "Observation type", "All observations");
    await frame.locator('[data-trajectory-record="item:image-1"] button[aria-expanded]').click();
    await frame.locator("img").waitFor();
    await frame.locator("img").evaluate((element) => {
      if (element.complete) return;
      return new Promise((resolve) => (element.onload = element.onerror = resolve));
    });
    assert.equal(await frame.locator("img").evaluate((element) => element.naturalWidth), 1);
    assert.equal(
      await frame.getByRole("button", { name: /^(Refresh|Newer records|Older records)$/ }).count(),
      3,
    );
    assert.equal(
      await frame.getByRole("button", { name: /^(Stop|Resume|Steer|Continue)$/ }).count(),
      0,
    );
    await frame.getByRole("button", { name: "Older records" }).click();
    await page.waitForFunction(() => window.reads.length === 1);
    assert.equal(await page.evaluate(() => window.reads[0].cursor), "next-fixture");
    await frame.getByText("1 recorded observations loaded on this page.").waitFor();
    assert.equal(await frame.locator("[data-trajectory-record]").count(), 1);
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
    const memoryInitial = {
      data: [
        {
          id: "mem_00000000000000000000000000000001",
          scope: "project",
          content: "Candidate <script> must render as text.",
          origin: "auto",
          status: "pending",
          pinned: false,
          createdAt: "2026-10-10T00:00:00Z",
          updatedAt: "2026-10-10T00:00:00Z",
        },
        {
          id: "mem_00000000000000000000000000000002",
          scope: "project",
          content: "Approved pinned fact",
          origin: "user",
          status: "active",
          pinned: true,
          createdAt: "2026-10-09T00:00:00Z",
          updatedAt: "2026-10-09T00:00:00Z",
        },
      ],
      nextCursor: "memory-next",
    };
    await page.evaluate(
      ({ html, initial }) => {
        window.continuation = { data: [] };
        void window.mount(html, initial);
      },
      { html: memoryHTML, initial: memoryInitial },
    );
    await frame
      .getByText("1 pending · 1 active on this page")
      .waitFor()
      .catch((error) => {
        throw new AggregateError(
          errors.map((message) => new Error(message)),
          "memory guest did not render",
          { cause: error },
        );
      });
    assert.equal(await frame.locator("article[data-memory-id]").count(), 2);
    assert.equal(await frame.locator("script").count(), 2);
    await frame.getByRole("combobox", { name: "Memory status" }).click();
    await frame.getByRole("option", { name: "Pending review", exact: true }).click();
    assert.equal(await frame.locator("article[data-memory-id]").count(), 1);
    await frame.getByRole("combobox", { name: "Memory status" }).click();
    await frame.getByRole("option", { name: "All statuses", exact: true }).click();
    await frame.getByRole("checkbox", { name: "Pinned only" }).check();
    assert.equal(await frame.locator("article[data-memory-id]").count(), 1);
    await frame.getByRole("checkbox", { name: "Pinned only" }).uncheck();
    await frame.getByRole("searchbox").fill("Candidate");
    assert.equal(await frame.locator("article[data-memory-id]").count(), 1);
    await frame.getByRole("searchbox").fill("");
    assert.equal(await page.evaluate(() => window.reads.length), 0);
    await reviewAppearance("memory");
    const loads = await page.evaluate(() => window.loads);
    await page.evaluate(() => {
      document.documentElement.style.setProperty("--color-accent", "#7344aa");
      window.frameController.updateTheme(window.captureTheme(document.documentElement, "dark", 18));
    });
    await expect(frame.locator("html")).toHaveAttribute("data-flame-ui-font-size", "18");
    assert.equal(
      await frame
        .locator("html")
        .evaluate((element) => element.style.getPropertyValue("--color-accent")),
      "#7344aa",
    );
    assert.equal(await page.evaluate(() => window.loads), loads);
    assert.equal(await page.evaluate(() => window.reads.length), 0);
    await page.setViewportSize({ width: 320, height: 700 });
    assert.equal(
      await frame.locator("html").evaluate((element) => element.scrollWidth <= element.clientWidth),
      true,
    );
    await frame.getByRole("button", { name: "Next page" }).click();
    await frame.getByText("0 memory items loaded on this page.").waitFor();
    assert.equal(await page.evaluate(() => window.reads[0].cursor), "memory-next");
    await page.evaluate(() => (window.readFailure = true));
    await frame.getByRole("button", { name: "Previous page" }).click();
    await frame.getByRole("alert").waitFor();
    assert.equal(await frame.locator("article[data-memory-id]").count(), 0);
    assert.equal(await frame.getByRole("button", { name: "Previous page" }).isEnabled(), true);
    await page.evaluate(() => (window.readFailure = false));
    await frame.getByRole("button", { name: "Previous page" }).click();
    await frame.getByText("2 memory items loaded on this page.").waitFor();
    await page.setViewportSize({ width: 1280, height: 720 });
    assert.equal(await frame.locator("article[data-memory-id]").count(), 2);
    await page.evaluate(() => window.dispose());
    await page.reload();
    await page.waitForFunction(() => window.mount);
    await page.setViewportSize({ width: 640, height: 720 });
    const usage = {
      total: {},
      sessions: 1,
      runs: 1,
      byProvider: [{ key: "provider <script> injected", runs: 1 }],
      byModel: [{ key: "provider/model", runs: 1, costUsd: 0 }],
      byDay: [{ key: "2026-10-10", runs: 1 }],
    };
    await page.evaluate(({ html, initial }) => void window.mount(html, initial, "dark"), {
      html: usageHTML,
      initial: usage,
    });
    await frame.getByText("Recorded usage loaded.").waitFor();
    await page.setViewportSize({ width: 320, height: 720 });
    const scrollable = frame.getByRole("region", { name: "Scrollable usage breakdown" });
    await scrollable.focus();
    await scrollable.press("ArrowRight");
    await expect
      .poll(() => scrollable.evaluate((element) => element.scrollLeft))
      .toBeGreaterThan(0);

    assert.equal(await page.evaluate(() => window.reads.length), 0);
    assert.equal(
      await frame.getByText("No finished Runs with reported usage in this period.").isVisible(),
      false,
    );
    assert.equal(await frame.locator("#totals dd").nth(1).textContent(), "0");
    assert.equal(await frame.locator("#totals dd").first().textContent(), "Not reported");
    assert.equal(
      await frame.locator("tbody").textContent(),
      "provider <script> injectedNot reported100000",
    );
    assert.equal(await frame.locator("script").count(), 2);
    await reviewAppearance("usage");
    await choose(frame, "Group by", "Model");
    assert.equal(await frame.locator("tbody tr").count(), 1);
    assert.equal(await frame.locator("tbody td").first().textContent(), "0.00");
    await frame.getByRole("searchbox", { name: "Filter" }).fill("absent");
    await frame.getByText("No matching rows.").waitFor();
    await frame.getByRole("searchbox", { name: "Filter" }).fill("");
    await page.evaluate(() => (window.readFailure = true));
    await frame.getByRole("button", { name: "Refresh" }).click();
    await frame.getByRole("alert").waitFor();
    assert.equal(await frame.locator("tbody tr").count(), 1);
    assert.equal(await frame.locator("#totals dd").first().textContent(), "Not reported");
    assert.equal(await page.evaluate(() => window.reads[0].cursor), undefined);
    await page.evaluate(() => (window.readFailure = false));
    await frame.getByRole("button", { name: "Refresh" }).click();
    await frame.getByText("Recorded usage loaded.").waitFor();
    await page.evaluate(() => window.dispose());
    await page.reload();
    await page.waitForFunction(() => window.mount);
    await page.evaluate(({ html }) => void window.mount(html, { total: {} }), { html: usageHTML });
    await frame.getByText("No finished Runs with reported usage in this period.").waitFor();
    assert.equal(await frame.locator("#breakdown").isVisible(), false);
    await page.evaluate(() => window.dispose());
    await page.reload();
    await page.waitForFunction(() => window.mount);
    for (const initial of [
      {
        type: "available",
        data: {
          skills: [
            {
              name: "verify",
              description: "<script>untrusted</script>",
              scope: "installation",
              installation: { installationId: "installed", digest: "release" },
            },
          ],
          diagnostics: [{ name: "broken", detail: "Invalid document" }],
        },
      },
      {
        type: "detail",
        data: {
          name: "verify",
          scope: "project",
          revision: "revision",
          path: "/remote/verify/SKILL.md",
          instructions: "<script>document.body.remove()</script>",
        },
      },
      { type: "library", data: { data: [{ name: "verify", lifecycle: "archived" }] } },
      {
        type: "review",
        data: {
          data: [
            {
              name: "verify",
              scope: "user",
              revision: "reviewed",
              instructions: "Review canonical facts.",
              description: "Proposed workflow",
              origin: "requested",
              revises: true,
            },
          ],
        },
      },
    ]) {
      await page.evaluate(({ html, initial }) => void window.mount(html, initial), {
        html: skillsHTML,
        initial,
      });
      await frame.getByText("Skill read completed.").waitFor();
      assert.equal(await frame.locator("article").count(), 1);
      assert.equal(await frame.locator("script").count(), 2);
      assert.equal(
        await frame.getByRole("button", { name: /Approve|Reject|Archive|Restore/ }).count(),
        0,
      );
      if (initial.type === "available") {
        await frame.getByText("Installation", { exact: true }).waitFor();
        await frame.getByText("Release", { exact: true }).waitFor();
        await frame.getByText("broken: Invalid document", { exact: true }).waitFor();
      }
      await reviewAppearance(`skills-${initial.type}`);
      if (initial.type === "detail") {
        await frame.getByText("Read instructions").click();
        await frame.getByText(initial.data.instructions, { exact: true }).waitFor();
      }
      await frame.getByRole("searchbox", { name: "Search this catalog" }).fill("absent");
      await frame.getByText("No matching Skills in this catalog.").waitFor();
      await frame.getByRole("searchbox", { name: "Search this catalog" }).fill("");
      await page.evaluate(() => (window.readFailure = true));
      await frame.getByRole("button", { name: "Refresh" }).click();
      await frame.getByRole("alert").waitFor();
      assert.equal(await frame.locator("article").count(), 1);
      assert.deepEqual(await page.evaluate(() => window.reads.map((read) => read.cursor)), [
        undefined,
      ]);
      await page.evaluate(() => (window.readFailure = false));
      await frame.getByRole("button", { name: "Refresh" }).click();
      await frame.getByText("Skill read completed.").waitFor();
      await page.evaluate(() => window.dispose());
      await page.reload();
      await page.waitForFunction(() => window.mount);
    }
    const schedules = {
      data: [
        {
          id: "sch_enabled",
          title: "Scheduled <script> review",
          instructions: "Review canonical changes",
          cron: "0 9 * * 1",
          enabled: true,
          revision: 3,
          nextRunAt: "2026-10-12T09:00:00Z",
          workspace: { path: "/runtime/project" },
        },
        {
          id: "sch_disabled",
          title: "Paused",
          instructions: "Never invent a firing time",
          cron: "0 3 * * *",
          enabled: false,
          revision: 2,
        },
      ],
      nextCursor: "schedule-next",
    };
    await page.evaluate(
      ({ html, initial }) => {
        window.continuation = { data: [] };
        void window.mount(html, initial, "dark");
      },
      { html: scheduleHTML, initial: schedules },
    );
    await frame.getByText("1 enabled · 1 disabled on this page").waitFor();
    assert.equal(await frame.locator("article").count(), 2);
    assert.equal(await frame.locator("script").count(), 2);
    assert.equal(
      await frame.locator("html").evaluate((element) => element.style.colorScheme),
      "dark",
    );
    await reviewAppearance("schedules");
    await choose(frame, "Schedule status", "Disabled");
    assert.equal(await frame.locator("article").count(), 1);
    assert.equal(await frame.getByText("Not scheduled", { exact: true }).count(), 1);
    assert.equal(await frame.getByText("Never fired", { exact: true }).count(), 1);
    await choose(frame, "Schedule status", "All statuses");
    await page.evaluate(() => (window.readFailure = true));
    await frame.getByRole("button", { name: "Next page" }).click();
    await frame.getByRole("alert").waitFor();
    assert.equal(await frame.locator("article").count(), 2);
    assert.equal(await frame.getByRole("button", { name: "Next page" }).isEnabled(), true);
    await page.evaluate(() => (window.readFailure = false));
    await frame.getByRole("button", { name: "Next page" }).click();
    await frame.getByText("0 schedules loaded on this page.").waitFor();
    assert.equal(await page.evaluate(() => window.reads[1].cursor), "schedule-next");
    assert.equal(await frame.getByRole("button", { name: /Run now|Delete|Enable/ }).count(), 0);
    await page.evaluate(() => window.dispose());
    await page.reload();
    await page.waitForFunction(() => window.mount);
    const diffHTML = await readFile(
      new URL("../../../plugins/diff/views/diff.html", import.meta.url),
      "utf8",
    );
    const diffInitial = {
      diff: {
        baseline: { type: "head", commit: "a".repeat(40) },
        truncated: true,
        files: [
          {
            path: "new<script>.ts",
            previousPath: "old.ts",
            status: "renamed",
            added: 0,
            removed: 0,
            rows: [
              { type: "hunk", text: "@@ -1 +1 @@" },
              { type: "context", leftLine: 1, rightLine: 1, code: "<script> must render as text" },
              { type: "deleted", leftLine: 2, code: "before" },
              { type: "added", rightLine: 2, code: "after".repeat(500) },
            ],
          },
          { path: "binary.dat", status: "untracked", binary: true, rows: [] },
        ],
      },
      focusPath: "new<script>.ts",
    };
    await page.evaluate(({ html, initial }) => void window.mount(html, initial, "dark"), {
      html: diffHTML,
      initial: diffInitial,
    });
    await frame.getByText("Workspace diff loaded.").waitFor();
    assert.equal(await frame.locator('[data-focused="true"]').count(), 1);
    assert.equal(
      await frame.locator("table code").first().textContent(),
      "<script> must render as text",
    );
    assert.match(
      await frame.locator("[data-diff-file] button[aria-expanded]").first().textContent(),
      /old.ts → new<script>.ts.*renamed.*\+0 −0/,
    );
    await frame.getByText(/This is a partial diff/).waitFor();
    assert.doesNotMatch(
      await frame.locator("[data-diff-file] button[aria-expanded]").last().textContent(),
      /\+0|−0/,
    );
    await frame.getByText("Binary content is not displayed.").waitFor();
    const fitsFrame = () =>
      frame.locator("html").evaluate((node) => node.scrollWidth <= node.clientWidth);
    assert.equal(await fitsFrame(), true, "long rows scroll inside the file, not the page");
    await reviewAppearance("diff");
    await choose(frame, "Layout", "Side by side");
    assert.equal(
      await frame.locator('table[data-layout="split"] tbody tr').last().locator("td").count(),
      4,
    );
    assert.equal(await fitsFrame(), true);
    await frame.getByRole("searchbox").fill("binary");
    assert.equal(await frame.locator("[data-diff-file]").count(), 1);
    await frame.getByRole("searchbox").fill("");
    await page.evaluate(() => (window.readFailure = true));
    await frame.getByRole("button", { name: "Refresh" }).click();
    await frame.getByRole("alert").waitFor();
    assert.equal(await frame.locator("[data-diff-file]").count(), 2);
    assert.equal(await page.evaluate(() => window.reads[0].cursor), undefined);
    await page.evaluate(() => (window.readFailure = false));
    await frame.getByRole("button", { name: "Refresh" }).click();
    await frame.getByText("Workspace diff loaded.").waitFor();
    assert.equal(await frame.getByRole("button", { name: /Stage|Restore|Edit|Delete/ }).count(), 0);
    await page.evaluate(() => window.dispose());
    await page.reload();
    await page.waitForFunction(() => window.mount);
    const resultHTML = await readFile(
      new URL("../../../plugins/tool-results/renderers/result.html", import.meta.url),
      "utf8",
    );
    const resultFixtures = [
      {
        name: "read",
        result: "recorded line\n<script>literal bytes</script>",
        range: { start: 41, end: 42 },
        expected: "literal bytes",
      },
      {
        name: "grep",
        result: JSON.stringify({
          hits: [{ path: "src/a<script>.ts", lineNumber: 12, snippet: "needle" }],
        }),
        expected: "src/a<script>.ts:12",
      },
      {
        name: "glob",
        result: JSON.stringify({ hits: [{ path: "src/one.ts" }, { path: "src/two.ts" }] }),
        expected: "src/one.ts",
      },
      {
        name: "acme.command",
        result: JSON.stringify({ output: "recorded command", exitCode: 0 }),
        expected: "recorded command",
      },
      {
        name: "apply_patch",
        result: JSON.stringify({ changes: [{ status: "moved", from: "old.ts", path: "new.ts" }] }),
        expected: "old.ts → new.ts",
      },
      {
        name: "http_request",
        result: JSON.stringify({
          status: 404,
          truncated: true,
          headers: { test: "header" },
          body: "recorded body",
        }),
        expected: "404",
      },
      {
        name: "web_fetch",
        result: JSON.stringify({ content: "<img src=escape>recorded page", format: "html" }),
        expected: "recorded page",
      },
      {
        name: "web_search",
        result: JSON.stringify({
          results: [
            {
              url: "https://example.invalid/reference",
              title: "Recorded reference",
              snippet: "source text",
            },
          ],
        }),
        expected: "Recorded reference",
      },
      {
        name: "list_skills",
        result: "<skill><name>inspect</name><description>Read recorded facts</description></skill>",
        expected: "Read recorded facts",
      },
      { name: "delegate_task", result: "Recorded child reply", expected: "Recorded child reply" },
      {
        name: "ask_user",
        result: JSON.stringify({ answer: "Accepted answer" }),
        expected: "Accepted answer",
      },
      { name: "lsp", result: "error src/a.ts:12 missing name", expected: "missing name" },
      { name: "search_memory", result: "1. Recorded memory", expected: "Recorded memory" },
      {
        name: "search_tools",
        result: "[reviews] list_reviews, update_review",
        expected: "list_reviews",
      },
    ];
    for (const fixture of resultFixtures) {
      await page.evaluate(
        ({ html, fixture }) =>
          void window.mount(html, {
            format: "flame.tool-result.v1",
            tool: {
              id: "call",
              runId: "run",
              fn: fixture.name,
              args: "{}",
              status: "ok",
              ...fixture,
            },
          }),
        { html: resultHTML, fixture },
      );
      await frame.getByText(/ok · Recorded result/).waitFor();
      assert.match(
        await frame.locator("#output").textContent(),
        new RegExp(fixture.expected.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")),
      );
      assert.equal(await frame.locator("img, a, script[src]").count(), 0);
      if (fixture.name === "shell")
        assert.equal(await frame.locator(".negative").textContent(), "Recorded failure");
      if (fixture.name === "read")
        assert.equal(await frame.locator(".number").first().textContent(), "41");
      await frame.getByRole("button", { name: "Refresh recorded result" }).click();
      await page.waitForFunction(() => window.reads.length === 1);
      assert.equal(await page.evaluate(() => window.reads[0].cursor), undefined);
      await page.evaluate(() => window.dispose());
      assert.equal(await page.locator("iframe").count(), 0);
    }
    await page.evaluate(
      ({ html }) =>
        void window.mount(html, {
          format: "flame.tool-result.v1",
          tool: {
            id: "call",
            runId: "run",
            name: "read",
            fn: "file",
            args: "{}",
            status: "ok",
            result: Array.from({ length: 100 }, (_, i) => `recorded line ${i}`).join("\n"),
          },
        }),
      { html: resultHTML },
    );
    await frame.getByText(/ok · Recorded result/).waitFor();
    assert.equal(await frame.locator(".row").count(), 40);
    await frame.getByRole("button", { name: /Show 40 more/ }).click();
    assert.equal(await frame.locator(".row").count(), 80);
    await frame.getByRole("searchbox").fill("line 99");
    assert.equal(await frame.locator(".row").count(), 1);
    await page.evaluate(() => (window.readFailure = true));
    await frame.getByRole("button", { name: "Refresh recorded result" }).click();
    await frame.getByRole("alert").waitFor();
    assert.equal(await frame.locator(".row").count(), 1);
    await page.evaluate(() => window.dispose());
    await page.reload();
    await page.waitForFunction(() => window.mount);
    await page.evaluate(
      (html) =>
        void window.mount(html, {
          format: "flame.tool-result.v999",
          tool: { status: "ok", result: "unrecognized" },
        }),
      resultHTML,
    );
    await page.waitForFunction(() => window.viewStatus?.type === "failure");
    assert.match(await page.evaluate(() => window.failure), /Unsupported recorded result format/);
    assert.equal(await page.locator("iframe").count(), 0);
    await page.reload();
    await page.waitForFunction(() => window.mount);
    const fileHTML = await readFile(
      new URL("../../../plugins/file-renderers/renderers/markdown.html", import.meta.url),
      "utf8",
    );
    const fileContent = [
      "# Captured Markdown",
      "",
      "**Strong text** and *emphasis* with `inline code`.",
      "",
      "- [x] Recorded task",
      "- Other item",
      "",
      "> Quoted observation",
      "",
      "| Column | Value |",
      "| --- | --- |",
      "| Fact | Recorded |",
      "",
      "```html",
      "<img onerror=escape()>",
      "```",
      "",
      `<script>window.markdownEscape = true</script>`,
      "",
      `[Remote link](${origin}/__escape)`,
      `![Image alternative](${origin}/__escape)`,
    ].join("\n");
    await page.evaluate(
      ({ html, content }) =>
        void window.mount(html, {
          format: "flame.file.v1",
          file: {
            cwd: "/runtime/workspace",
            path: "notes.md",
            content,
            startLine: 1,
            totalLines: content.split("\n").length,
          },
        }),
      { html: fileHTML, content: fileContent },
    );
    await page.waitForFunction(() => window.viewStatus?.type === "ready");
    await frame.locator("article").getByRole("heading", { name: "Captured Markdown" }).waitFor();
    assert.equal(await frame.locator("article strong").innerText(), "Strong text");
    assert.equal(await frame.locator("article table tbody tr").count(), 1);
    assert.equal(await frame.locator("article blockquote").innerText(), "Quoted observation");
    assert.equal(await frame.locator("img, a, object, iframe, [onclick], [onerror]").count(), 0);
    assert.equal(await frame.locator("html").evaluate(() => window.markdownEscape), undefined);
    const capturedFile = await frame.locator("#output").innerText();
    assert.match(capturedFile, /Image alternative/);
    assert.match(capturedFile, /<script>window.markdownEscape/);
    await frame.getByRole("button", { name: "Refresh captured file" }).click();
    await page.waitForFunction(() => window.reads.length === 1);
    await expect(frame.getByRole("button", { name: "Refresh captured file" })).toBeEnabled();
    assert.equal(await frame.locator("#output").innerText(), capturedFile);
    assert.equal(await page.evaluate(() => window.reads[0].cursor), undefined);
    await page.evaluate(() => (window.readFailure = true));
    await frame.getByRole("button", { name: "Refresh captured file" }).click();
    await frame.getByRole("alert").waitFor();
    assert.equal(await frame.locator("#output").innerText(), capturedFile);
    await page.evaluate(() => window.dispose());
    await page.reload();
    await page.waitForFunction(() => window.mount);
    await page.evaluate(
      (html) =>
        void window.mount(html, {
          format: "flame.file.v1",
          file: { path: "partial.md", content: "partial", startLine: 41, totalLines: 100 },
        }),
      fileHTML,
    );
    await page.waitForFunction(() => window.viewStatus?.type === "failure");
    assert.match(await page.evaluate(() => window.failure), /Unsupported captured file format/);
    assert.equal(await page.locator("iframe").count(), 0);
    await page.reload();
    await page.waitForFunction(() => window.mount);
    const diagnosticsHTML = await readFile(
      new URL("../../../plugins/diagnostics/views/diagnostics.html", import.meta.url),
      "utf8",
    );
    const observations = {
      format: "flame.client-diagnostics.v1",
      source: "currentClient",
      spans: {
        rows: [
          {
            id: "span",
            name: "client.rpc",
            durationMillis: 0,
            status: "error",
            attrs: { unsafe: "<img src=x onerror=alert(1)>" },
          },
        ],
        omittedFromRead: 5,
      },
      metrics: {
        rows: [
          {
            id: "metric",
            name: "client.render",
            kind: "histogram",
            count: 3,
            sum: 42,
            p50: 10,
            p95: 20,
            avg: 14,
            unit: "ms",
            attrs: {},
          },
        ],
        omittedFromRead: 0,
      },
      logs: {
        rows: [{ id: "log", severity: "WARN", body: "Recorded local log", attrs: {} }],
        omittedFromRead: 1,
      },
    };
    await page.evaluate(({ html, initial }) => void window.mount(html, initial), {
      html: diagnosticsHTML,
      initial: observations,
    });
    await frame.getByRole("heading", { name: "Client diagnostics" }).waitFor();
    await expect(frame.getByRole("status")).toContainText("5 omitted");
    await reviewAppearance("diagnostics");
    await frame.locator("#output button[aria-expanded]").click();
    assert.match(await frame.locator("pre").textContent(), /<img src=x/);
    assert.equal(await frame.locator("img").count(), 0);
    await choose(frame, "Telemetry signal", "Metrics");
    await frame.locator("#output button[aria-expanded]").click();
    await expect(frame.locator("#output dd")).toHaveCount(5);
    await choose(frame, "Telemetry signal", "Logs");
    await frame.getByText("WARN · Recorded local log", { exact: true }).waitFor();
    await frame.getByRole("button", { name: "Refresh" }).click();
    await page.waitForFunction(() => window.reads.length === 1);
    assert.equal(await page.evaluate(() => window.reads[0].cursor), undefined);
    assert.equal(await frame.getByRole("button", { name: "Clear" }).count(), 0);
    await page.evaluate(() => (window.readFailure = true));
    await expect(frame.getByRole("button", { name: "Refresh" })).toBeEnabled();
    await frame.getByRole("button", { name: "Refresh" }).click();
    await frame.getByRole("alert").waitFor();
    await frame.getByText("WARN · Recorded local log", { exact: true }).waitFor();
    await page.evaluate(() => window.dispose());
    await page.reload();
    await page.waitForFunction(() => window.mount);
    await page.evaluate(
      ({ html, initial }) =>
        void window.mount(html, {
          ...initial,
          spans: { rows: [null], omittedFromRead: 0 },
        }),
      { html: diagnosticsHTML, initial: observations },
    );
    await page.waitForFunction(() => window.viewStatus?.type === "failure");
    assert.match(await page.evaluate(() => window.failure), /could not be displayed/);
    assert.equal(await page.locator("iframe").count(), 0);
    await page.reload();
    await page.waitForFunction(() => window.mount);
    const galleryHTML = await readFile(
      new URL("../../../plugins/icon-gallery/views/icon-gallery.html", import.meta.url),
      "utf8",
    );
    await page.evaluate(
      (html) => void window.mount(html, { format: "flame.static.v1" }),
      galleryHTML,
    );
    await frame.getByRole("heading", { name: "Flame icon gallery" }).waitFor();
    await reviewAppearance("icon-gallery");
    await frame.getByRole("searchbox", { name: "Filter icons" }).fill("file-diff");
    await frame.getByText("FileDiff", { exact: true }).waitFor();
    await expect(frame.getByText("FileText", { exact: true })).toBeHidden();
    assert.ok((await frame.locator("article:visible svg path").count()) > 0);
    await frame.getByRole("searchbox", { name: "Filter icons" }).fill("no-such-flame-glyph");
    await expect(frame.getByRole("status")).toContainText(/0 of \d+ icons/);
    assert.equal(await page.evaluate(() => window.reads.length), 0);
    await page.evaluate(() => window.dispose());
    assert.equal(await page.locator("iframe").count(), 0);
    await page.reload();
    await page.waitForFunction(() => window.mount);
    const malicious = `<!doctype html><script>addEventListener('message',async e=>{
     if(e.source!==parent||e.data?.type!=='flame.view.connect.v3')return;
     try{parent.parent.document.documentElement.dataset.guestEscape='true'}catch{}
     try{localStorage.setItem('guestEscape','true')}catch{}
     new Image().src='${origin}/__escape';
     try{await fetch('${origin}/__escape')}catch{}
     let peer;
     try{peer=new RTCPeerConnection({iceServers:[{urls:'stun:127.0.0.1:${peer.address().port}'}]});peer.createDataChannel('escape');await peer.setLocalDescription()}catch{}
     await new Promise(resolve=>setTimeout(resolve,600));peer?.close();
     e.ports[0].postMessage({type:'tools.invoke',sessionId:'other'});
    });parent.postMessage('flame.view.ready.v3','*');</script>`;
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
