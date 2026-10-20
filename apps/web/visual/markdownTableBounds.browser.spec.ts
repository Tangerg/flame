import { expect, test, type Locator, type Page } from "./test";
for (const [width, theme, fontSize] of [
  [1440, "light", 14],
  [320, "dark", 18],
] as const) {
  test(`expanded table controls and headings remain reachable while reading ${width}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      `/visual/?fixture=agent&state=table-reading&theme=${theme}&font-size=${fontSize}&sidebar=collapsed`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    const expand = page.getByRole("button", { name: "Expand table", exact: true });
    const transcript = (await page.locator(".msg-scroll-viewport").boundingBox())!;
    const control = (await expand.boundingBox())!;
    const center = transcript.y + transcript.height / 2;
    await page.mouse.move(transcript.x + transcript.width / 2, center);
    await page.mouse.wheel(0, control.y - center);
    await expect(expand).toBeInViewport({ ratio: 1 });
    const cells = await page
      .locator("[data-markdown-table] th, [data-markdown-table] td")
      .allTextContents();
    await expand.click();
    const dialog = page.getByRole("dialog", { name: "Table preview", exact: true });
    await expect(dialog.getByRole("row")).toHaveCount(81);
    await expect(dialog.locator("th, td")).toHaveText(cells);
    const close = dialog.getByRole("button", { name: "Close table preview", exact: true });
    const frame = (await dialog.boundingBox())!;
    const firstRow = dialog.getByRole("row").nth(1);
    await page.mouse.move(frame.x + frame.width / 2, frame.y + frame.height / 2);
    await page.mouse.wheel(0, 600);
    await expect
      .poll(async () => {
        const row = (await firstRow.boundingBox())!;
        return row.y + row.height;
      })
      .toBeLessThan(frame.y);
    const screenshot = test.info().outputPath("expanded-table-after-scroll.png");
    await page.screenshot({ path: screenshot });
    await test.info().attach("expanded-table-after-scroll", { path: screenshot });
    await expect(close).toBeInViewport({ ratio: 1 });
    await expect(
      dialog.getByRole("columnheader", { name: "Observation", exact: true }),
    ).toBeInViewport({ ratio: 1 });
    const reader = dialog.getByRole("region", { name: "Table content", exact: true });
    await expect(reader).toBeFocused();
    const top = await reader.evaluate((node) => node.scrollTop);
    await page.keyboard.down("ArrowDown");
    try {
      await expect.poll(() => reader.evaluate((node) => node.scrollTop)).toBeGreaterThan(top);
    } finally {
      await page.keyboard.up("ArrowDown");
    }
    if (width === 320) {
      await page.keyboard.down("ArrowRight");
      try {
        await expect.poll(() => reader.evaluate((node) => node.scrollLeft)).toBeGreaterThan(0);
      } finally {
        await page.keyboard.up("ArrowRight");
      }
      await expect(close).toBeInViewport({ ratio: 1 });
    }
    await page.keyboard.press("Shift+Tab");
    await expect(close).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(dialog).toHaveCount(0);
    await expect(expand).toBeFocused();
    await page.keyboard.press("Enter");
    await expect(reader).toBeFocused();
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await expect(expand).toBeFocused();
  });
}

async function bounds(container: Locator) {
  return container.evaluate((node) => {
    const box = (element: Element) => {
      const { left, right, top, bottom, width, height } = element.getBoundingClientRect();
      return { left, right, top, bottom, width, height };
    };
    const markdown = node.closest(".md")!;
    const scroller = node.querySelector<HTMLElement>(".md-table-scroller")!;
    const table = node.querySelector("table")!;
    const actions = node.querySelector(".md-table-actions")!;
    const buttons = Array.from(actions.querySelectorAll("button"));
    const clip = box(scroller);
    const overlaps: { action: string | null; text: string }[] = [];
    const textNodes = document.createTreeWalker(table, NodeFilter.SHOW_TEXT);
    for (let text = textNodes.nextNode(); text; text = textNodes.nextNode()) {
      if (!text.textContent?.trim()) continue;
      const range = document.createRange();
      range.selectNodeContents(text);
      for (const rect of range.getClientRects()) {
        const left = Math.max(rect.left, clip.left);
        const right = Math.min(rect.right, clip.right);
        const top = Math.max(rect.top, clip.top);
        const bottom = Math.min(rect.bottom, clip.bottom);
        if (right <= left || bottom <= top) continue;
        for (const button of buttons) {
          const action = box(button);
          if (
            left < action.right &&
            right > action.left &&
            top < action.bottom &&
            bottom > action.top
          ) {
            overlaps.push({ action: button.getAttribute("aria-label"), text: text.textContent });
          }
        }
      }
    }
    return {
      markdown: box(markdown),
      container: box(node),
      scroller: clip,
      table: box(table),
      actions: box(actions),
      buttons: buttons.map(box),
      scrollWidth: scroller.scrollWidth,
      clientWidth: scroller.clientWidth,
      overflowX: getComputedStyle(scroller).overflowX,
      overlaps,
    };
  });
}

async function wheelTableIntoView(page: Page, table: Locator) {
  const transcript = page.locator(".msg-scroll-viewport");
  const frame = (await transcript.boundingBox())!;
  const box = (await table.boundingBox())!;
  const center = frame.y + frame.height / 2;
  await page.mouse.move(frame.x + frame.width / 2, center);
  await page.mouse.wheel(0, box.y + box.height / 2 - center);
  await expect(table).toBeInViewport({ ratio: 1 });
}

async function readTableWithKeyboard(page: Page, container: Locator) {
  await container.evaluate((node) => node.scrollIntoView({ block: "center" }));
  const scroller = container.locator(".md-table-scroller");
  const range = await scroller.evaluate((node) => node.scrollWidth - node.clientWidth);
  expect(range).toBeGreaterThan(1);
  await container.focus();
  await page.keyboard.press("Tab");
  await expect(container.getByRole("button", { name: "Expand table", exact: true })).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(container.getByRole("button", { name: "Copy table", exact: true })).toBeFocused();
  await page.keyboard.press("Tab");
  await expect(scroller).toBeFocused();
  await expect(scroller).toHaveAttribute("role", "region");
  await expect(scroller).toHaveAccessibleName("Table content");
  await expect(scroller).toHaveAttribute("data-focus-inset", "");
  const transcript = page.locator(".msg-scroll-viewport");
  const outerTop = await transcript.evaluate((node) => node.scrollTop);
  const before = await scroller.evaluate((node) => node.scrollLeft);
  await page.keyboard.down("ArrowRight");
  try {
    await expect.poll(() => scroller.evaluate((node) => node.scrollLeft)).toBeGreaterThan(before);
  } finally {
    await page.keyboard.up("ArrowRight");
  }
  expect(await transcript.evaluate((node) => node.scrollTop)).toBe(outerTop);
  await page.keyboard.press("Shift+Tab");
  await expect(container.getByRole("button", { name: "Copy table", exact: true })).toBeFocused();
  await page.keyboard.press("Shift+Tab");
  await expect(container.getByRole("button", { name: "Expand table", exact: true })).toBeFocused();
}

for (const [width, fontSize, theme] of [
  [1472, 14, "light"],
  [320, 18, "dark"],
] as const) {
  test(`Markdown table and revealed actions stay inside the answer measure ${width} ${theme}`, async ({
    page,
  }) => {
    await page.setViewportSize({ width, height: 900 });
    await page.goto(
      `/visual/?fixture=agent&state=long-content&theme=${theme}&font-size=${fontSize}&sidebar=collapsed`,
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await page.evaluate(() => document.fonts.ready);
    const table = page.locator("[data-markdown-table]").filter({ hasText: "Boundary" });
    await wheelTableIntoView(page, table);
    await table.hover();
    const actions = table.locator(".md-table-actions");
    await expect(actions).toHaveCSS("opacity", "1");
    const readings = [{ state: "hover", ...(await bounds(table)) }];
    const copy = table.getByRole("button", { name: "Copy table", exact: true });
    await copy.focus();
    await page.mouse.move(0, 0);
    await expect(copy).toBeFocused();
    await expect(actions).toHaveCSS("opacity", "1");
    readings.push({ state: "focus", ...(await bounds(table)) });
    await test.info().attach("markdown-table-bounds", {
      body: JSON.stringify(readings, null, 2),
      contentType: "application/json",
    });
    for (const reading of readings) {
      for (const [name, box] of Object.entries({
        container: reading.container,
        scroller: reading.scroller,
        actions: reading.actions,
        ...Object.fromEntries(reading.buttons.map((button, index) => [`action ${index}`, button])),
      })) {
        expect
          .soft(box.left, `${reading.state} ${name} left`)
          .toBeGreaterThanOrEqual(reading.markdown.left);
        expect
          .soft(box.right, `${reading.state} ${name} right`)
          .toBeLessThanOrEqual(reading.markdown.right);
      }
      if (reading.scrollWidth <= reading.clientWidth) {
        expect.soft(reading.table.left).toBeGreaterThanOrEqual(reading.markdown.left);
        expect.soft(reading.table.right).toBeLessThanOrEqual(reading.markdown.right);
        if (width > 1000) {
          expect
            .soft(reading.actions.right, `${reading.state} actions share the table's trailing edge`)
            .toBeLessThanOrEqual(reading.table.right);
          expect
            .soft(
              reading.actions.bottom,
              `${reading.state} actions have their own zone above the reader`,
            )
            .toBeLessThanOrEqual(reading.scroller.top);
          expect.soft(reading.container.bottom).toBe(reading.scroller.bottom);
        }
      } else {
        expect(reading.overflowX).toBe("auto");
      }
      expect.soft(reading.overlaps, `${reading.state} actions cover table text`).toEqual([]);
    }
  });
}

test("an overflowing canonical table remains keyboard readable as the answer resizes", async ({
  page,
}) => {
  await page.setViewportSize({ width: 320, height: 1200 });
  await page.goto("/visual/?fixture=agent&state=narrative&theme=dark&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(() => document.fonts.ready);
  const table = page.locator("[data-markdown-table]").filter({ hasText: "Strategy" });
  await readTableWithKeyboard(page, table);

  await page.setViewportSize({ width: 1472, height: 900 });
  const scroller = table.locator(".md-table-scroller");
  await expect
    .poll(() => scroller.evaluate((node) => node.scrollWidth - node.clientWidth))
    .toBeLessThanOrEqual(1);
  await expect(scroller).not.toHaveAttribute("tabindex");
  await expect(scroller).not.toHaveAttribute("role");
  await expect(scroller).not.toHaveAttribute("aria-label");
  await table.focus();
  await page.keyboard.press("Tab");
  await expect(table.getByRole("button", { name: "Expand table", exact: true })).toBeFocused();
});

test("a wide authored table remains keyboard readable in the desktop Dock reading column", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1472, height: 900 });
  await page.goto(
    "/visual/?fixture=workspace&state=dock-light&agent-state=narrative&theme=light&font-size=18",
  );
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(() => document.fonts.ready);
  const resize = page.getByRole("separator", { name: "Resize right workspace", exact: true });
  await resize.focus();
  await resize.press("End");
  await expect(resize).toHaveAttribute(
    "aria-valuenow",
    (await resize.getAttribute("aria-valuemax"))!,
  );

  const original = page.locator("[data-markdown-table]").filter({ hasText: "Strategy" });
  const originalScroller = original.locator(".md-table-scroller");
  expect(
    await originalScroller.evaluate((node) => node.scrollWidth - node.clientWidth),
  ).toBeLessThanOrEqual(1);
  await expect(originalScroller).not.toHaveAttribute("tabindex");
  await original.focus();
  await page.keyboard.press("Tab");
  await expect(original.getByRole("button", { name: "Expand table", exact: true })).toBeFocused();

  await page.evaluate(async () => {
    if (!location.pathname.startsWith("/visual/"))
      throw new Error("An isolated fixture is required");
    const fixturePath = "/visual/agentSessionSnapshots.ts";
    const storePath = "/src/features/agent/adapters/agentStore.ts";
    const projectionPath = "/src/features/agent/application/session/sessionSnapshot.ts";
    const fixture = (await import(fixturePath)) as typeof import("./agentSessionSnapshots");
    const { useAgentStore } = await import(storePath);
    const { projectAgentSessionSnapshot } = await import(projectionPath);
    const snapshot = structuredClone(fixture.RUNTIME_AGENT_SESSION_SNAPSHOTS.narrative);
    const answer = snapshot.items.find(
      (item) =>
        item.type === "agentMessage" &&
        item.content?.some(
          (part) => part.type === "text" && part.text.includes("Strategy comparison"),
        ),
    );
    if (answer?.type !== "agentMessage") throw new Error("The accepted answer is required");
    answer.content = [
      {
        type: "text",
        text: [
          "## Recorded observations",
          "",
          "Compare the accepted measurements before continuing.",
          "",
          "| Observation_01 | Observation_02 | Observation_03 | Observation_04 | Observation_05 |",
          "| --- | --- | --- | --- | --- |",
          "| Accepted | Accepted | Accepted | Accepted | Final observation |",
          "",
          "The explanation continues after the recorded evidence.",
        ].join("\n"),
      },
    ];
    const store = useAgentStore.getState();
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
  const wide = page.locator("[data-markdown-table]").filter({ hasText: "Observation_01" });
  await readTableWithKeyboard(page, wide);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
});
