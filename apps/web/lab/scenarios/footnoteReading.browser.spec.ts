import AxeBuilder from "@axe-core/playwright";
import { en } from "@/lib/i18n/locales/en";
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };
import { catalog } from "../catalog";
import { previewURL, readSelection } from "../selection";
import { expect, test } from "./test";
import { expectLabReady } from "../readiness";

for (const environment of [
  { locale: "en", theme: "light", fontSize: 14, messages: en },
  { locale: "zh-CN", theme: "dark", fontSize: 18, messages: zh },
]) {
  test(`isolated reference reading retains a distinct return path in ${environment.locale}`, async ({
    page,
    context,
  }, testInfo) => {
    const messages: Record<string, string> = environment.messages;
    const selection = readSelection(
      new URLSearchParams({
        panel: "business",
        story: "business-references",
        variant: "long",
        locale: environment.locale,
        theme: environment.theme,
        "font-size": String(environment.fontSize),
      }).toString(),
      catalog,
    );
    await page.setViewportSize({ width: 320, height: 720 });
    await page.goto(previewURL(catalog.story(selection.storyId), selection, "reference-reading"));
    await expectLabReady(page, { timeout: 5_000 });
    const references = page.locator("[data-footnote-ref]");
    const back = page.getByRole("link", {
      name: messages["message.markdown.backReference"]!.replace("{{reference}}", "1-2"),
      exact: true,
    });
    await expect(references).toHaveCount(2);
    await expect(references.first()).toHaveCSS("text-decoration-line", "none");
    await expect(back).toHaveCSS("text-decoration-line", "underline");
    await references.last().focus();
    await page.keyboard.press("Enter");
    await expect(page.locator("[data-footnotes] li")).toBeInViewport();
    await back.focus();
    await expect(back).toBeInViewport({ ratio: 1 });
    const audit = await new AxeBuilder({ page })
      .withRules(["color-contrast", "link-in-text-block"])
      .analyze();
    await testInfo.attach("reference-reading-audit", {
      body: Buffer.from(JSON.stringify(audit, null, 2)),
      contentType: "application/json",
    });
    expect(audit.violations).toEqual([]);
    await testInfo.attach("reference-note", {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });
    await page.keyboard.press("Enter");
    await expect(references.last()).toBeFocused();
    await testInfo.attach("returned-reference-bounds", {
      body: Buffer.from(
        JSON.stringify(
          await references.last().evaluate((element) => ({
            bounds: element.getBoundingClientRect().toJSON(),
            viewport: { width: innerWidth, height: innerHeight },
            margin: getComputedStyle(element).scrollMarginBlockStart,
            scroll: document.scrollingElement?.scrollTop,
          })),
        ),
      ),
      contentType: "application/json",
    });
    await expect(references.last()).toBeInViewport({ ratio: 1 });
    await testInfo.attach("reference-return", {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });
    expect(context.pages()).toHaveLength(1);
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}

for (const [width, fontSize, theme] of [
  [1472, 14, "light"],
  [800, 18, "dark"],
] as const) {
  test(`supporting notes stay within their answer and return to the chosen reference ${theme}`, async ({
    page,
    context,
  }, testInfo) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto(
      `/lab/fixture.html?fixture=agent&state=reference-notes&theme=${theme}&font-size=${fontSize}`,
    );
    await expectLabReady(page);
    const draft = page.getByRole("textbox");
    await draft.fill("Keep the draft while reading references.");
    const answers = page.locator(".md").filter({ has: page.locator("[data-footnotes]") });
    await expect(answers).toHaveCount(2);
    const relations = await answers.evaluateAll((nodes) =>
      nodes.map((node) => {
        const references = [...node.querySelectorAll<HTMLAnchorElement>("[data-footnote-ref]")];
        const backReferences = [
          ...node.querySelectorAll<HTMLAnchorElement>("[data-footnote-backref]"),
        ];
        const labelId = references[0]?.getAttribute("aria-describedby") ?? "";
        const label = document.getElementById(labelId);
        const bounds = label?.getBoundingClientRect();
        const footer = node.querySelector<HTMLElement>("[data-footnotes]")!;
        const localTarget = (link: HTMLAnchorElement) => {
          const id = link.getAttribute("href")!.slice(1);
          const target = document.getElementById(id);
          return target !== null && node.contains(target);
        };
        return {
          ids: [...node.querySelectorAll("[id]")].map((element) => element.id),
          references: references.length,
          backReferences: backReferences.length,
          targetsStayLocal: [...references, ...backReferences].every(localTarget),
          currentDocument: [...references, ...backReferences].every((link) => !link.target),
          label: label?.textContent,
          hiddenLabel:
            bounds &&
            bounds.width <= 1 &&
            bounds.height <= 1 &&
            getComputedStyle(label!).clipPath !== "none",
          bodySize: Number.parseFloat(getComputedStyle(node).fontSize),
          noteSize: Number.parseFloat(getComputedStyle(footer).fontSize),
          overflow: footer.scrollWidth - footer.clientWidth,
        };
      }),
    );
    const ids = relations.flatMap((relation) => relation.ids);
    expect(new Set(ids).size).toBe(ids.length);
    for (const relation of relations) {
      expect(relation.references).toBe(2);
      expect(relation.backReferences).toBe(2);
      expect(relation.targetsStayLocal).toBe(true);
      expect(relation.currentDocument).toBe(true);
      expect(relation.label).toBe("Footnotes");
      expect(relation.hiddenLabel).toBe(true);
      expect(relation.noteSize).toBeLessThan(relation.bodySize);
      expect(relation.overflow).toBe(0);
    }

    const answer = answers.last();
    const references = answer.locator("[data-footnote-ref]");
    const target = (await references.first().getAttribute("href"))!.slice(1);
    await references.first().focus();
    await references.first().press("Enter");
    await expect.poll(() => page.evaluate(() => location.hash.slice(1))).toBe(target);
    const definition = answer.locator("[data-footnotes] li").first();
    await expect(definition).toBeInViewport();
    const back = answer.getByRole("link", { name: "Back to reference 1-2", exact: true });
    await expect(back).toHaveCSS("text-decoration-line", "underline");
    await expect(references.first()).toHaveCSS("text-decoration-line", "none");
    await back.focus();
    await testInfo.attach("production-note", {
      body: await page.screenshot({ animations: "disabled" }),
      contentType: "image/png",
    });
    await back.press("Enter");
    await expect(references.last()).toBeFocused();
    await expect(references.last()).toBeInViewport();
    await expect(draft).toHaveValue("Keep the draft while reading references.");
    expect(context.pages()).toHaveLength(1);

    const external = answer.getByRole("link", { name: "protocol guide", exact: true });
    await expect(external).toHaveAttribute("target", "_blank");
    await expect(external).toHaveAttribute("rel", "noopener noreferrer");
    await expect(external).toHaveAttribute("title", "Read the recorded contract");
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}
