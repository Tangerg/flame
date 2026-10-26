import { expect, test } from "./test";
for (const [theme, fontSize, width] of [
  ["light", 14, 1472],
  ["dark", 18, 800],
] as const) {
  test(`Chinese paragraphs preserve semantic separation at ${fontSize}px in ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      `/visual/?fixture=agent&state=long-content&theme=${theme}&font-size=${fontSize}`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await page.evaluate(() => document.fonts.ready);
    const section = page.getByRole("heading", { name: "中文长回答", exact: true });
    await section.scrollIntoViewIfNeeded();
    const paragraphs = await section.evaluate((heading) => {
      const rows = [];
      for (
        let node = heading.nextElementSibling;
        node?.tagName === "P";
        node = node.nextElementSibling
      ) {
        const bounds = node.getBoundingClientRect();
        rows.push({
          text: node.textContent,
          direction: node.getAttribute("dir"),
          top: bounds.top,
          bottom: bounds.bottom,
          fontSize: Number.parseFloat(getComputedStyle(node).fontSize),
        });
      }
      return rows;
    });
    expect(paragraphs).toHaveLength(3);
    for (const paragraph of paragraphs) {
      expect(paragraph.direction).toBe("auto");
      expect(paragraph.fontSize).toBe(fontSize);
    }
    expect(paragraphs[1]!.text).toContain("runtime.Commit");
    for (let index = 1; index < paragraphs.length; index++) {
      expect(paragraphs[index]!.top - paragraphs[index - 1]!.bottom).toBe(fontSize);
    }
    expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
  });
}

test("expanded reasoning preserves the same Markdown line and list rhythm as the answer", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 1100 });
  await page.goto("/visual/?fixture=agent&state=idle&theme=light&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(() => document.fonts.ready);
  await page.evaluate(async () => {
    if (!location.pathname.startsWith("/visual/"))
      throw new Error("An isolated fixture is required");
    const fixturePath = "/visual/agentSessionSnapshots.ts";
    const storePath = "/visual/main.tsx";
    const projectionPath = "/src/features/agent/application/session/sessionSnapshot.ts";
    const fixture = (await import(fixturePath)) as typeof import("./agentSessionSnapshots");
    const { visualClient } = await import(storePath);
    const { projectAgentSessionSnapshot } = await import(projectionPath);
    const snapshot = structuredClone(fixture.RUNTIME_AGENT_SESSION_SNAPSHOTS.idle);
    const prompt = snapshot.items.find((item) => item.type === "userMessage");
    const answer = snapshot.items.find((item) => item.type === "agentMessage");
    if (!prompt || !answer)
      throw new Error("The fixture must contain accepted prompt and answer items");
    const source = "First line\nSecond line\n\nNext paragraph\n\n- one\n- two";
    snapshot.items = [
      prompt,
      {
        type: "reasoning",
        id: "item_multiline_reasoning",
        runId: answer.runId,
        status: "completed",
        createdAt: answer.createdAt,
        text: source,
      },
      { ...answer, id: "item_multiline_answer", content: [{ type: "text", text: source }] },
    ];
    const store = visualClient.agent.projection.store.getState();
    const refresh = store.beginViewRefresh("ses_visual", true);
    if (
      !refresh ||
      !store.commitViewRefresh(
        "ses_visual",
        refresh,
        projectAgentSessionSnapshot(fixture.projectRuntimeAgentSnapshot(snapshot)),
      )
    )
      throw new Error("The fixture snapshot could not be installed");
  });

  await page.getByRole("button", { name: "Thought", exact: true }).click();
  const reasoning = page.locator('[data-slot="reasoning-scroller"] .md');
  await expect(reasoning.locator("p")).toHaveCount(2);
  await expect(reasoning.locator("li")).toHaveText(["one", "two"]);
  const answer = page.locator(".md").filter({ hasText: "First line" }).last();
  await expect(answer.locator("p")).toHaveCount(2);
  await expect(answer.locator("li")).toHaveText(["one", "two"]);

  const measured = await page.evaluate(() => {
    const inspect = (root: Element) => {
      const paragraphs = [...root.querySelectorAll(":scope > p")].map((node) =>
        node.getBoundingClientRect(),
      );
      const list = root.querySelector("ul")!.getBoundingClientRect();
      const items = [...root.querySelectorAll("li")].map((node) => node.getBoundingClientRect());
      return {
        lineHeight: Number.parseFloat(getComputedStyle(root).lineHeight),
        height: root.getBoundingClientRect().height,
        paragraphHeights: paragraphs.map((paragraph) => paragraph.height),
        paragraphGap: paragraphs[1]!.top - paragraphs[0]!.bottom,
        paragraphToListGap: list.top - paragraphs[1]!.bottom,
        listHeight: list.height,
        listTopInset: items[0]!.top - list.top,
        itemHeights: items.map((item) => item.height),
        itemGap: items[1]!.top - items[0]!.bottom,
      };
    };
    const port = document.querySelector<HTMLElement>('[data-slot="reasoning-scroller"]')!;
    const reasoning = port.querySelector(".md")!;
    const answer = [...document.querySelectorAll(".md")].find(
      (node) => !port.contains(node) && node.textContent?.includes("First line"),
    )!;
    return {
      reasoning: inspect(reasoning),
      answer: inspect(answer),
      reasoningOverflow: port.scrollHeight - port.clientHeight,
    };
  });
  expect(measured.answer.paragraphHeights[0]).toBe(measured.answer.lineHeight * 2);
  expect(measured.reasoning.paragraphHeights[0]).toBe(measured.reasoning.lineHeight * 2);
  expect(measured.reasoning).toEqual(measured.answer);
  expect(measured.reasoningOverflow).toBe(0);
});
