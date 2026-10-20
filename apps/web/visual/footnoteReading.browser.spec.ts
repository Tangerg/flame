import { expect, test } from "./test";
for (const [width, fontSize, theme] of [
  [1472, 14, "light"],
  [800, 18, "dark"],
] as const) {
  test(`supporting notes stay within their answer and return to the chosen reference ${theme}`, async ({
    page,
    context,
  }) => {
    await page.setViewportSize({ width, height: 1000 });
    await page.goto(
      `/visual/?fixture=agent&state=reference-notes&theme=${theme}&font-size=${fontSize}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
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
    await back.focus();
    await back.press("Enter");
    await expect(references.last()).toBeFocused();
    await expect(references.last()).toBeInViewport();
    expect(context.pages()).toHaveLength(1);

    const external = answer.getByRole("link", { name: "protocol guide", exact: true });
    await expect(external).toHaveAttribute("target", "_blank");
    await expect(external).toHaveAttribute("rel", "noopener noreferrer");
    await expect(external).toHaveAttribute("title", "Read the recorded contract");
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}
