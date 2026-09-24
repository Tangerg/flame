import { mkdirSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath } from "node:url";
import type { CDPSession } from "@playwright/test";
import { expect, test, type Page } from "./test";
import { VISUAL_REVIEW_VIEWPORT } from "./workspaceFixtureStates";

const SAMPLES = Number(process.env.FLAME_PERF_SAMPLES ?? 5);
const REVIEW_FILES = 200;
const REPORT_DIR = join(dirname(fileURLToPath(import.meta.url)), "../../.cache/perf");

type Metrics = Record<string, number>;

interface Probe {
  longTasks: number[];
  events: { name: string; duration: number }[];
}

declare global {
  interface Window {
    __flamePerf: Probe;
  }
}

const results = new Map<string, Metrics[]>();

function record(scenario: string, metrics: Metrics): void {
  const samples = results.get(scenario) ?? [];
  samples.push(metrics);
  results.set(scenario, samples);
}

function quantile(sorted: number[], q: number): number {
  if (sorted.length === 0) return 0;
  const index = Math.min(sorted.length - 1, Math.ceil(q * sorted.length) - 1);
  return sorted[Math.max(0, index)]!;
}

function spread(values: number[]) {
  const sorted = values.toSorted((a, b) => a - b);
  return {
    p50: quantile(sorted, 0.5),
    p90: quantile(sorted, 0.9),
    max: sorted.at(-1) ?? 0,
  };
}

test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const probe: Probe = { longTasks: [], events: [] };
    window.__flamePerf = probe;
    new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) probe.longTasks.push(entry.duration);
    }).observe({ type: "longtask", buffered: true });
    new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        probe.events.push({ name: entry.name, duration: entry.duration });
      }
    }).observe({ type: "event", buffered: true, durationThreshold: 16 } as PerformanceObserverInit);
  });
});

test.afterAll(() => {
  const summary = Object.fromEntries(
    [...results].map(([scenario, samples]) => [
      scenario,
      Object.fromEntries(
        Object.keys(samples[0] ?? {}).map((key) => [key, spread(samples.map((s) => s[key]!))]),
      ),
    ]),
  );
  mkdirSync(REPORT_DIR, { recursive: true });
  const stamp = new Date().toISOString().replace(/[:.]/g, "-");
  const file = join(REPORT_DIR, `interaction-${stamp}.json`);
  writeFileSync(file, `${JSON.stringify({ samples: SAMPLES, summary }, null, 2)}\n`);
  console.log(`\nperf report (${SAMPLES} samples): ${file}`);
  const cell = (value: number) => value.toFixed(1).padStart(8);
  for (const [scenario, metrics] of Object.entries(summary)) {
    console.log(`\n${scenario}`);
    for (const [key, { p50, p90, max }] of Object.entries(metrics)) {
      console.log(`  ${key.padEnd(18)} p50 ${cell(p50)}  p90 ${cell(p90)}  max ${cell(max)}`);
    }
  }
});

async function cdp(page: Page): Promise<CDPSession> {
  const session = await page.context().newCDPSession(page);
  await session.send("Performance.enable");
  return session;
}

async function engine(session: CDPSession): Promise<Metrics> {
  const { metrics } = await session.send("Performance.getMetrics");
  return Object.fromEntries(metrics.map(({ name, value }) => [name, value]));
}

async function resetProbe(page: Page): Promise<void> {
  await page.evaluate(() => {
    window.__flamePerf.longTasks.length = 0;
    window.__flamePerf.events.length = 0;
  });
}

async function measure(
  page: Page,
  session: CDPSession,
  act: () => Promise<void>,
): Promise<Metrics> {
  await resetProbe(page);
  const before = await engine(session);
  const started = Date.now();
  await act();
  await page.evaluate(
    () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
  );
  const wall = Date.now() - started;
  const after = await engine(session);
  const probe = await page.evaluate(() => window.__flamePerf);
  const seconds = (key: string) => ((after[key] ?? 0) - (before[key] ?? 0)) * 1000;
  return {
    wallMs: wall,
    taskMs: seconds("TaskDuration"),
    scriptMs: seconds("ScriptDuration"),
    layoutMs: seconds("LayoutDuration"),
    styleMs: seconds("RecalcStyleDuration"),
    longTaskMs: probe.longTasks.reduce((sum, value) => sum + value, 0),
    longTaskMaxMs: Math.max(0, ...probe.longTasks),
    inputToPaintMaxMs: Math.max(0, ...probe.events.map((event) => event.duration)),
    heapMb: (after.JSHeapUsedSize ?? 0) / 2 ** 20,
  };
}

async function measureLoad(page: Page, load: () => Promise<void>): Promise<Metrics> {
  await load();
  await page.evaluate(
    () => new Promise((resolve) => requestAnimationFrame(() => requestAnimationFrame(resolve))),
  );
  const probe = await page.evaluate(() => ({
    readyMs: performance.now(),
    longTasks: window.__flamePerf.longTasks,
  }));
  const session = await cdp(page);
  const heap = (await engine(session)).JSHeapUsedSize ?? 0;
  await session.detach();
  return {
    readyMs: probe.readyMs,
    longTaskMs: probe.longTasks.reduce((sum, value) => sum + value, 0),
    longTaskMaxMs: Math.max(0, ...probe.longTasks),
    heapMb: heap / 2 ** 20,
  };
}

const REVIEW_URL = `/visual/?fixture=workspace&state=dock-review&theme=light&review-files=${REVIEW_FILES}`;

test.describe.configure({ mode: "serial", timeout: 1_800_000 });

test(`a ${REVIEW_FILES}-file review opens, folds and scrolls`, async ({ page }) => {
  for (let sample = 0; sample < SAMPLES; sample++) {
    await page.setViewportSize(VISUAL_REVIEW_VIEWPORT);
    const diffFiles = page.locator("[data-diff-file]");
    record(
      "review: open",
      await measureLoad(page, async () => {
        await page.goto(REVIEW_URL);
        await page.locator("html[data-visual-ready]").waitFor();
        await expect(diffFiles).toHaveCount(REVIEW_FILES);
      }),
    );
    const session = await cdp(page);

    record(
      "review: collapse all",
      await measure(page, session, async () => {
        await page.getByRole("button", { name: "Collapse all files" }).click();
        await expect(page.getByRole("button", { name: "Expand all files" })).toBeVisible();
      }),
    );

    record(
      "review: expand all",
      await measure(page, session, async () => {
        await page.getByRole("button", { name: "Expand all files" }).click();
        await expect(page.getByRole("button", { name: "Collapse all files" })).toBeVisible();
      }),
    );

    await diffFiles.first().hover();
    record(
      "review: scroll 40 wheels",
      await measure(page, session, async () => {
        for (let step = 0; step < 40; step++) await page.mouse.wheel(0, 900);
      }),
    );

    record(
      "review: resize sweep x10",
      await measure(page, session, async () => {
        for (let step = 0; step < 10; step++) {
          await page.setViewportSize({
            width: step % 2 === 0 ? 1280 : VISUAL_REVIEW_VIEWPORT.width,
            height: VISUAL_REVIEW_VIEWPORT.height,
          });
          await page.evaluate(() => new Promise((resolve) => requestAnimationFrame(resolve)));
        }
      }),
    );
    await session.detach();
  }
});

test("a long transcript loads and takes typing", async ({ page }) => {
  for (let sample = 0; sample < SAMPLES; sample++) {
    await page.setViewportSize({ width: 1280, height: 800 });
    record(
      "long transcript: open",
      await measureLoad(page, async () => {
        await page.goto("/visual/?fixture=agent&state=long-content&theme=light");
        await page.locator("html[data-visual-ready]").waitFor();
        await expect(page.getByRole("img", { name: "Diagram" })).toBeVisible();
      }),
    );

    const session = await cdp(page);
    const composer = page.getByRole("textbox", { name: "Message composer" });
    await composer.click();
    record(
      "long transcript: type 40 chars",
      await measure(page, session, async () => {
        await page.keyboard.type("Measure the input path while history is long.".slice(0, 40), {
          delay: 25,
        });
      }),
    );
    await session.detach();
  }
});
