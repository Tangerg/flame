import { expect, test, type Locator } from "./test";
import { en } from "@/lib/i18n/locales/en";
import { expectLabReady } from "../readiness";
async function expectBelowWindowControl(content: Locator, toggle: Locator) {
  await expect(content).toBeVisible();
  const control = (await toggle.boundingBox())!;
  const bounds = (await content.boundingBox())!;
  expect(bounds.y).toBeGreaterThanOrEqual(control.y + control.height);
}

async function expectCentreHit(control: Locator) {
  expect(
    await control.evaluate((node) => {
      const bounds = node.getBoundingClientRect();
      const target = document.elementFromPoint(
        bounds.x + bounds.width / 2,
        bounds.y + bounds.height / 2,
      );
      return target !== null && node.contains(target);
    }),
  ).toBe(true);
}

test("Workspace layout observation is opt-in and preserves the material and draft", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1472, height: 1000 });
  await page.goto(
    "/lab/fixture.html?fixture=workspace&state=dock-skills-catalog&locale=en&theme=light&font-size=14&motion=off",
  );
  await expectLabReady(page);
  const observe = page.getByRole("checkbox", { name: "Observe layout", exact: true });
  const output = page.getByTestId("workspace-layout-observation");
  const readingOutput = page.getByTestId("workspace-reading-observation");
  await expect(observe).not.toBeChecked();
  await expect(output).toHaveText("");
  await expect(readingOutput).toHaveText("");
  await observe.click();
  await expect(observe).toBeChecked();
  await expect
    .poll(async () => {
      const value = await output.textContent();
      return value ? (JSON.parse(value) as { window: { width: number } }).window.width : null;
    })
    .toBe(1472);
  await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
  const composer = page.getByRole("textbox", { name: "Message composer", exact: true });
  const draft = "Unsent layout-observation draft — 保留原始输入。";
  await composer.fill(draft);
  const search = page.getByRole("searchbox", { name: "Search this catalog", exact: true });
  const viewport = page.locator('[data-slot="workspace-view-body"]:visible');
  const reportedHeight = async () => {
    const value = await output.textContent();
    return value ? (JSON.parse(value) as { viewport: { total: number } }).viewport.total : null;
  };
  await search.focus();
  const initialHeight = await viewport.evaluate((node) => node.scrollHeight);
  await expect.poll(reportedHeight).toBe(initialHeight);
  await search.fill("runtime");
  await expect(
    page.getByRole("heading", { name: "40 of 80 available Skills", exact: true }),
  ).toBeVisible();
  const filteredFrames = await output.evaluate(async (node) => {
    const frames = [];
    await new Promise(requestAnimationFrame);
    for (let frame = 0; frame < 8; frame += 1) {
      await new Promise(requestAnimationFrame);
      const observed = JSON.parse(node.textContent!) as { viewport: { total: number } };
      const reader = document.querySelector<HTMLElement>('[data-slot="workspace-view-body"]')!;
      frames.push({ reported: observed.viewport.total, measured: reader.scrollHeight });
    }
    return frames;
  });
  await test.info().attach("workspace-layout-filtered-frames", {
    body: JSON.stringify({ initialHeight, filteredFrames }),
    contentType: "application/json",
  });
  for (const frame of filteredFrames) {
    expect(frame.measured).toBeLessThan(initialHeight);
    expect(frame.reported).toBe(frame.measured);
  }
  await search.fill("");
  await expect.poll(reportedHeight).toBe(initialHeight);
  await search.fill("runtime");
  await expect.poll(reportedHeight).toBe(filteredFrames[0]!.measured);
  const reader = await viewport.elementHandle();
  const reading = viewport.getByRole("button").filter({
    has: page.getByText("runtime-review-035", { exact: true }),
  });
  await reading.focus();
  await expect(reading).toBeFocused();
  await expect
    .poll(async () => {
      const value = await output.textContent();
      return value
        ? ((JSON.parse(value) as { reading: { name: string } | null }).reading?.name ?? "")
        : "";
    })
    .toContain("runtime-review-035");
  await expect(output).not.toContainText(draft);
  await expect(readingOutput).toContainText("runtime-review-035");
  await expect(readingOutput).not.toContainText(draft);
  await expect(composer).toHaveValue(draft);
  await expect(search).toHaveValue("runtime");
  expect(await viewport.evaluate((node, before) => node === before, reader)).toBe(true);

  await page.getByRole("button", { name: "Expand the workspace fixture sidebar" }).click();
  await observe.click();
  await expect(observe).not.toBeChecked();
  await expect(output).toHaveText("");
  await expect(readingOutput).toHaveText("");
  await search.fill("");
  await page.setViewportSize({ width: 1400, height: 900 });
  const retired = await output.evaluate(async (node) => {
    const frames = [];
    for (let frame = 0; frame < 3; frame += 1) {
      await new Promise(requestAnimationFrame);
      frames.push(node.textContent);
    }
    return frames;
  });
  expect(retired).toEqual(["", "", ""]);
  await expect(readingOutput).toHaveText("");
  await expect(composer).toHaveValue(draft);
  expect(await viewport.evaluate((node, before) => node === before, reader)).toBe(true);
});

for (const theme of ["light", "dark"]) {
  for (const width of [320, 1120]) {
    for (const view of ["subagents", "usage", "diagnostics"]) {
      test(`full-width ${view} keeps its content below window controls ${theme} ${width}`, async ({
        page,
      }) => {
        await page.setViewportSize({ width, height: 568 });
        const query = new URLSearchParams({
          fixture: "workspace",
          state: "full-view",
          "full-view": view === "subagents" ? view : `package:visual:${view}`,
          "agent-state": "delegated",
          "font-size": "18",
          theme,
        });
        await page.goto(`/lab/fixture.html?${query}`);
        await expectLabReady(page);
        await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
        const toggle = page.getByRole("button", {
          name: "Expand the workspace fixture sidebar",
        });
        await expect(toggle).toHaveAttribute("aria-expanded", "false");
        const header = page.locator('[data-slot="workspace-window-header"]');
        await expect(header).toBeInViewport({ ratio: 1 });
        for (const action of await header.getByRole("button").all()) await expectCentreHit(action);

        if (view === "subagents") {
          const panel = page.getByRole("region", { name: "Subagents", exact: true });
          await expectBelowWindowControl(
            panel.getByText("6 subagents · 1 running", { exact: true }),
            toggle,
          );
          await panel
            .getByRole("button", { name: "Audit Agent Framework ownership Needs input" })
            .click();
          await expectBelowWindowControl(
            panel.getByRole("button", { name: "All subagents", exact: true }),
            toggle,
          );
          return;
        }

        const panel = page.getByRole("region", {
          name: view === "usage" ? "Usage" : "Client diagnostics",
          exact: true,
        });
        if (view === "usage") {
          for (const details of await panel
            .getByRole("button", { name: "Token and coverage details" })
            .all())
            await details.click();
        } else {
          await page.evaluate(async () => {
            const modulePath = "/src/lib/observability/stores.ts";
            const { useTelemetryStore } = await import(modulePath);
            useTelemetryStore.getState().ingestSpans([
              {
                id: "window-chrome-evidence",
                traceId: "trace_window_chrome",
                name: "Inspect original request evidence",
                kind: "client",
                startMs: 1785484800000,
                durationMillis: 124,
                status: "ok",
                attrs: { path: `/Users/visual/${"retained-evidence/".repeat(30)}request.json` },
              },
            ]);
          });
          await panel.getByRole("button", { name: "Refresh", exact: true }).click();
          await panel
            .getByRole("region", { name: "Local observations", exact: true })
            .getByRole("button", { expanded: false })
            .first()
            .click();
        }
        const viewport = page.locator('[data-slot="workspace-view-body"]').filter({ has: panel });
        expect(await viewport.evaluate((node) => node.scrollHeight > node.clientHeight)).toBe(true);
        await viewport.evaluate((node) => {
          node.scrollTop = node.scrollHeight;
        });
        const controls = panel.locator('[data-slot="panel-controls"]');
        await expectBelowWindowControl(controls, toggle);
        await expect(controls).toBeInViewport({ ratio: 1 });
        await expect(header).toBeInViewport({ ratio: 1 });
        const action =
          view === "usage"
            ? panel.getByRole("searchbox", { name: "Filter sources", exact: true })
            : panel.getByRole("button", { name: "Refresh", exact: true });
        await action.click();
        await expect(action).toBeFocused();
        await expectCentreHit(action);
      });
    }
  }
}

test.describe("coarse pointer window controls", () => {
  test.use({ hasTouch: true, isMobile: true });

  for (const theme of ["light", "dark"]) {
    test(`window actions keep full touch targets beside native controls ${theme}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width: 320, height: 568 });
      await page.goto(
        `/lab/fixture.html?fixture=workspace&state=full-view&full-view=subagents&agent-state=delegated&theme=${theme}&font-size=18`,
      );
      await expectLabReady(page);
      expect(await page.evaluate(() => matchMedia("(pointer: coarse)").matches)).toBe(true);
      await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
      const toggle = page.getByRole("button", { name: "Expand the workspace fixture sidebar" });
      const native = (await toggle.boundingBox())!;
      const header = page.locator('[data-slot="workspace-window-header"]');
      const bounds = (await header.boundingBox())!;
      for (const action of await header.getByRole("button").all()) {
        const box = (await action.boundingBox())!;
        expect(box.width).toBeGreaterThanOrEqual(44);
        expect(box.height).toBeGreaterThanOrEqual(44);
        expect(box.x).toBeGreaterThanOrEqual(native.x + native.width);
        expect(box.x + box.width).toBeLessThanOrEqual(320);
        expect(box.y).toBeGreaterThanOrEqual(bounds.y);
        expect(box.y + box.height).toBeLessThanOrEqual(bounds.y + bounds.height);
        await expectCentreHit(action);
      }
    });
  }
});

for (const theme of ["light", "dark"]) {
  for (const width of [320, 1120]) {
    test(`Settings reserves native chrome without a phantom sidebar ${theme} ${width}`, async ({
      page,
    }) => {
      await page.setViewportSize({ width, height: 568 });
      await page.goto(
        `/lab/fixture.html?fixture=workspace&state=settings&theme=${theme}&font-size=18`,
      );
      await expectLabReady(page);
      const header = page.locator('[data-slot="workspace-window-header"]');
      await expect(page.locator(".agent-shell")).toHaveAttribute("data-sidebar", "absent");
      await expect(page.getByRole("button", { name: /workspace fixture sidebar/ })).toHaveCount(0);
      const search = page.getByRole("searchbox", { name: en["settings.searchPlaceholder"]! });
      for (const gutter of [78, 0]) {
        await page.evaluate((value) => {
          document.documentElement.style.setProperty("--window-controls-gutter", `${value}px`);
        }, gutter);
        const padding = await header.evaluate((node) => {
          const style = getComputedStyle(node);
          return {
            start: parseFloat(style.paddingInlineStart),
            end: parseFloat(style.paddingInlineEnd),
          };
        });
        expect(padding.start).toBe(Math.max(gutter, padding.end));
        const window = (await header.boundingBox())!;
        expect((await search.boundingBox())!.y).toBeGreaterThanOrEqual(window.y + window.height);
        const close = header.getByRole("button", { name: "Close", exact: true });
        await expect(close).toBeInViewport({ ratio: 1 });
        await expectCentreHit(close);
      }
      await header.getByRole("button", { name: "Close", exact: true }).click();
      await expect(page.getByTestId("active-main-view")).toHaveText("");
      await expect(
        page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }),
      ).toBeVisible();
      await expect(page.getByRole("textbox", { name: "Message composer" })).toBeVisible();
    });
  }
}
