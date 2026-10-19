import { expect, test, type Locator } from "./test";
import { en } from "@/lib/i18n/locales/en";

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
        await page.goto(`/visual/?${query}`);
        await page.locator("html[data-visual-ready]").waitFor();
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
            panel.getByRole("button", { name: "Back to subagents", exact: true }),
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
            ? panel.getByRole("searchbox", { name: "Filter", exact: true })
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
        `/visual/?fixture=workspace&state=full-view&full-view=subagents&agent-state=delegated&theme=${theme}&font-size=18`,
      );
      await page.locator("html[data-visual-ready]").waitFor();
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
      await page.goto(`/visual/?fixture=workspace&state=settings&theme=${theme}&font-size=18`);
      await page.locator("html[data-visual-ready]").waitFor();
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
