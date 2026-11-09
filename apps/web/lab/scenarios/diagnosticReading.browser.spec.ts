import { expect, test } from "./test";
import { expectLabReady } from "../readiness";
import { en as diagnosticsEn } from "../../../../plugins/diagnostics/component/locales/en";
import { zh as diagnosticsZh } from "../../../../plugins/diagnostics/component/locales/zh";
import { en } from "../../src/lib/i18n/locales/en";
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };
test("a log has a concise identity and preserves its complete multiline reading body", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 1000 });
  await page.goto(
    "/lab/fixture.html?fixture=workspace&state=dock-diagnostics-recorded&theme=dark&font-size=18",
  );
  await expectLabReady(page);
  const headline = "Workspace preview could not be refreshed.";
  const body = `\n${headline}\n\nError: the file changed after the accepted request.\n    at refresh (/Users/visual/scope/apps/web/src/features/workspace/ui/files/FileWorkspace.tsx:89:7)\nThe last accepted preview remains available.`;
  const panel = page.getByRole("region", { name: "Client diagnostics", exact: true });
  await panel.getByRole("button", { name: "Refresh", exact: true }).click();
  await panel.getByRole("radio", { name: /^Logs ·/ }).click();
  const observations = panel.getByRole("region", { name: "Local observations", exact: true });
  const summary = observations.getByRole("button", { name: new RegExp(headline) });
  await expect(summary).toContainText(headline);
  await expect(summary).not.toContainText("at refresh");
  await expect(summary).not.toContainText("The last accepted preview");
  await summary.click();
  const paragraph = observations.locator("p").filter({ hasText: "at refresh" });
  await expect(paragraph).toBeVisible();
  expect(await paragraph.textContent()).toBe(body);
  const rendered = await paragraph.evaluate((node) => {
    const selection = window.getSelection()!;
    const range = document.createRange();
    range.selectNodeContents(node);
    selection.removeAllRanges();
    selection.addRange(range);
    const text = selection.toString();
    selection.removeAllRanges();
    return { text, overflow: node.scrollWidth - node.clientWidth };
  });
  expect(rendered.text).toBe(body);
  expect(rendered.overflow).toBe(0);
  await observations.getByRole("button", { name: "Raw observation", exact: true }).click();
  await expect(
    observations.getByRole("region", { name: "Local observation details" }),
  ).toContainText(JSON.stringify(body));
  expect(await panel.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
});

for (const browserName of ["chromium", "webkit"] as const) {
  const verify = test.extend({ browserName });
  for (const profile of [
    {
      locale: "en",
      theme: "light",
      fontSize: 14,
      width: 1120,
      height: 720,
      labels: diagnosticsEn,
      messages: en,
      draft: "Continue after reading the original observations.",
      continuation: " Keep the current task.",
    },
    {
      locale: "zh",
      theme: "dark",
      fontSize: 18,
      width: 640,
      height: 400,
      labels: diagnosticsZh,
      messages: zh as Record<string, string>,
      draft: "阅读原始观测后继续。",
      continuation: "保留当前任务。",
    },
  ] as const) {
    verify(
      `recorded Diagnostics preserve evidence, local clearing and task return in ${browserName}/${profile.locale}`,
      async ({ page }, testInfo) => {
        const { locale, theme, fontSize, width, height, labels, messages } = profile;
        const named = (key: string) => ({ name: labels[key]!, exact: true });
        const capture = async (name: string) =>
          testInfo.attach(`diagnostics-${name}-${browserName}-${locale}`, {
            body: await page.screenshot({ animations: "disabled" }),
            contentType: "image/png",
          });
        const route = `/lab/fixture.html?fixture=workspace&state=dock-diagnostics-recorded&locale=${locale}&theme=${theme}&font-size=${fontSize}`;
        await page.setViewportSize({ width, height });
        await page.goto(route);
        await expectLabReady(page);
        await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
        const composer = page.getByRole("textbox", {
          name: messages["composer.input.label"],
          exact: true,
        });
        await composer.fill(profile.draft);
        await page
          .getByRole("button", {
            name: messages[width === 640 ? "dock.action.openFull" : "dock.action.expand"],
            exact: true,
          })
          .click();
        const panel = page
          .getByRole("region", named("panel.title"))
          .and(page.locator('[data-slot="panel"]'));
        const observations = panel.getByRole("region", named("observations.title"));
        const signal = (key: string, count: number) =>
          panel.getByRole("radio", { name: `${labels[`signal.${key}`]} · ${count}`, exact: true });
        await expect(signal("spans", 3)).toBeChecked();
        await expect(signal("metrics", 2)).toBeVisible();
        await expect(signal("logs", 2)).toBeVisible();
        for (const [name, status] of [
          ["read", "unset"],
          ["decode", "ok"],
          ["refresh", "error"],
        ]) {
          const row = observations.getByRole("button", {
            name: new RegExp(`workspace\\.preview\\.${name}`),
          });
          await expect(
            row.getByText(labels[`span.status.${status}`]!, { exact: true }),
          ).toBeVisible();
        }
        await capture("traces");
        const read = observations.getByRole("button", { name: /workspace\.preview\.read/ });
        await read.click();
        await observations.getByRole("button", named("raw.title")).click();
        const raw = observations.getByRole("region", named("raw.label"));
        const original = (await raw.textContent())!;
        expect(JSON.parse(original)).toMatchObject({
          id: "0000000000000001",
          status: "unset",
          traceId: "0123456789abcdef0123456789abcdef",
        });
        await raw.focus();
        await page.keyboard.press("ControlOrMeta+A");
        expect(await raw.evaluate(() => window.getSelection()!.toString())).toBe(original);
        await page.keyboard.press("ArrowRight");
        await page.setViewportSize({ width: 320, height: 600 });
        await expect(raw).toBeFocused();
        await expect
          .poll(() =>
            raw.evaluate((node) => {
              const box = node.getBoundingClientRect();
              const controls = node
                .closest('[data-slot="panel"]')!
                .querySelector('[data-slot="panel-controls"]')!
                .getBoundingClientRect();
              return Math.max(
                -box.left,
                controls.bottom - box.top,
                box.right - innerWidth,
                box.bottom - innerHeight,
              );
            }),
          )
          .toBeLessThanOrEqual(1);
        expect(await panel.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
        expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(
          0,
        );
        await capture("raw-320");
        await page.setViewportSize({ width, height });
        const refresh = panel.getByRole("button", named("refresh"));
        await refresh.click();
        await expect(refresh).toBeFocused();
        await expect(read).toHaveAttribute("aria-expanded", "true");
        expect(await raw.textContent()).toBe(original);
        await signal("spans", 3).press("ArrowRight");
        await expect(signal("metrics", 2)).toBeChecked();
        const histogram = observations.getByRole("button", { name: /workspace\.preview\.latency/ });
        await histogram.click();
        const fact = (key: string) =>
          observations
            .getByText(labels[`metric.${key}`]!, { exact: true })
            .locator("..")
            .locator("dd");
        for (const [key, value] of [
          ["count", "12"],
          ["sum", "840 ms"],
          ["p50", "50 ms"],
          ["p95", "100 ms"],
          ["avg", "70 ms"],
        ])
          await expect(fact(key!)).toHaveText(value!);
        await observations.locator("dl").first().scrollIntoViewIfNeeded();
        await capture("histogram");
        await histogram.click();
        await observations.getByRole("button", { name: /workspace\.preview\.opens/ }).click();
        await expect(fact("count")).toHaveText("24 {preview}");
        await expect(fact("sum")).toHaveText("24 {preview}");
        await signal("metrics", 2).press("ArrowRight");
        await expect(signal("logs", 2)).toBeChecked();
        const log = observations.getByRole("button", {
          name: /Workspace preview could not be refreshed\./,
        });
        await log.click();
        const body =
          "\nWorkspace preview could not be refreshed.\n\nError: the file changed after the accepted request.\n    at refresh (/Users/visual/scope/apps/web/src/features/workspace/ui/files/FileWorkspace.tsx:89:7)\nThe last accepted preview remains available.";
        expect(
          await observations.locator("p").filter({ hasText: "at refresh" }).textContent(),
        ).toBe(body);
        await observations.getByRole("button", named("raw.title")).click();
        expect(JSON.parse((await raw.textContent())!)).toMatchObject({ id: "reading-log", body });
        await raw.focus();
        await page.keyboard.press("ControlOrMeta+A");
        expect(await raw.evaluate(() => window.getSelection()!.toString())).toBe(
          await raw.textContent(),
        );
        await page.keyboard.press("ArrowRight");
        await capture("log-source");
        await refresh.click();
        await expect(log).toHaveAttribute("aria-expanded", "true");
        await expect(signal("logs", 2)).toBeChecked();
        const actions = panel.getByRole("button", named("actions.label"));
        await actions.focus();
        await page.keyboard.press("Enter");
        const clear = page.getByRole("menuitem", named("actions.clear"));
        await expect(clear).toBeFocused();
        await page.keyboard.press("Enter");
        await expect(actions).toBeFocused();
        await expect(signal("spans", 0)).toBeChecked();
        await expect(signal("metrics", 0)).toBeVisible();
        await expect(signal("logs", 0)).toBeVisible();
        await expect(
          panel.getByText(labels["observations.empty.title"]!, { exact: true }),
        ).toBeVisible();
        await refresh.click();
        await expect(signal("logs", 0)).toBeVisible();
        await capture("cleared");
        await page.keyboard.press("Escape");
        await expect(composer).toBeVisible();
        await expect(composer).toHaveValue(profile.draft);
        await composer.focus();
        await page.keyboard.press("ControlOrMeta+A");
        await page.keyboard.press("ArrowRight");
        await page.keyboard.type(profile.continuation);
        await expect(composer).toHaveValue(profile.draft + profile.continuation);
        await capture("continued-draft");
        for (const recorded of [true, false]) {
          await page.goto(
            recorded ? route : route.replace("dock-diagnostics-recorded", "dock-diagnostics"),
          );
          await expectLabReady(page);
          await page
            .getByRole("button", { name: "Collapse the workspace fixture sidebar" })
            .click();
          await page
            .getByRole("button", {
              name: messages[width === 640 ? "dock.action.openFull" : "dock.action.expand"],
              exact: true,
            })
            .click();
          await expect(signal("spans", recorded ? 3 : 0)).toBeChecked();
          await expect(signal("metrics", recorded ? 2 : 0)).toBeVisible();
          await expect(signal("logs", recorded ? 2 : 0)).toBeVisible();
        }
      },
    );
  }
}
