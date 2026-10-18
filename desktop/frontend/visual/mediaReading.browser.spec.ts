import { expect, test } from "./test";
import { LONG_RESPONSE_MARKDOWN } from "./agentSessionSnapshots";

test("file image references keep encoded payloads in Source and out of the reading flow", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 1000 });
  const remoteRequests: string[] = [];
  page.on("request", (request) => {
    if (request.url().startsWith("https://tracker.example/")) remoteRequests.push(request.url());
  });
  await page.goto("/visual/?fixture=workspace&state=dock-files&theme=dark&font-size=18");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.getByRole("treeitem", { name: "reading-guide.md", exact: true }).click();
  const view = page.locator('[data-dock-view-id="file"]');
  const reading = view.locator('[data-slot="document-body"]');
  await expect(reading.getByRole("heading", { name: "Architecture review" })).toBeVisible();
  await expect(reading.getByRole("img", { name: "Inline architecture" })).toBeVisible();
  await expect(reading.getByRole("img", { name: "Inline detail" })).toBeVisible();
  await expect(
    reading.getByRole("img", { name: "Inline architecture" }),
  ).toHaveAccessibleDescription("Embedded image data");
  await expect(reading.getByText("Embedded image data", { exact: true })).toHaveCount(2);
  await expect(reading).not.toContainText("data:image/");
  await expect(reading).toContainText("https://tracker.example/pixel.png");
  await expect(reading.locator("img,a,iframe,object,embed")).toHaveCount(0);
  expect(await reading.evaluate((node) => node.scrollWidth - node.clientWidth)).toBe(0);
  expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);

  await view.getByRole("tab", { name: "Source", exact: true }).click();
  const payload = LONG_RESPONSE_MARKDOWN.match(/data:image\/svg\+xml;base64,[^)]+/)![0];
  await expect(view.locator('[data-quote-source="file"]')).toContainText(payload);
  await view.getByRole("tab", { name: "Preview", exact: true }).click();
  await expect(reading.getByText("Embedded image data", { exact: true })).toHaveCount(2);
  expect(remoteRequests).toEqual([]);
});

test("media-only paragraphs own image spacing across wrapped and unavailable images", async ({
  page,
}) => {
  const gallery = page.locator('[data-markdown-image-grid="true"]');
  const spacing = () =>
    gallery.evaluate((group) => {
      const children = Array.from(group.children);
      if (children.length !== 2) throw new Error("Canonical gallery must contain two images");
      const first = children[0]!.getBoundingClientRect();
      const second = children[1]!.getBoundingClientRect();
      const sameRow = first.top === second.top;
      const style = getComputedStyle(group);
      const reading = group.closest(".msg-scroll-viewport")!;
      return {
        sameRow,
        gap: sameRow ? second.left - first.right : second.top - first.bottom,
        parentGap: Number.parseFloat(sameRow ? style.columnGap : style.rowGap),
        margins: children.map((child) => {
          const style = getComputedStyle(child);
          return [style.marginBlockStart, style.marginBlockEnd];
        }),
        readingOverflow: reading.scrollWidth - reading.clientWidth,
        pageOverflow: document.documentElement.scrollWidth - innerWidth,
      };
    });

  for (const width of [1472, 1120]) {
    await page.setViewportSize({ width, height: 720 });
    await page.goto(
      "/visual/?fixture=workspace&state=dock-trajectory&agent-state=long-content&theme=light&font-size=14",
    );
    await page.locator("html[data-visual-ready]").waitFor();
    await expect(gallery.locator("img")).toHaveCount(2);
    for (const image of await gallery.locator("img").all()) {
      await image.evaluate((image: HTMLImageElement) => image.decode());
    }
    await gallery.scrollIntoViewIfNeeded();
    const measured = await spacing();
    expect(measured.sameRow).toBe(width === 1472);
    expect(measured.gap).toBe(measured.parentGap);
    expect(measured.margins).toEqual([
      ["0px", "0px"],
      ["0px", "0px"],
    ]);
    expect(measured.readingOverflow).toBe(0);
    expect(measured.pageOverflow).toBe(0);
  }

  await gallery
    .locator("img")
    .first()
    .evaluate((image) => image.dispatchEvent(new Event("error")));
  const unavailable = gallery.getByRole("img", { name: "Inline architecture", exact: true });
  await expect(unavailable).toContainText("Image unavailable");
  await expect(gallery.locator("img")).toHaveCount(1);
  const failed = await spacing();
  expect(failed.margins).toEqual([
    ["0px", "0px"],
    ["0px", "0px"],
  ]);
  expect(failed.gap).toBe(failed.parentGap);
  expect(failed.readingOverflow).toBe(0);
  expect(failed.pageOverflow).toBe(0);
});

test("protocol portraits keep their frame fitted and decode failures preserve surrounding rhythm", async ({
  page,
}) => {
  await page.setViewportSize({ width: 1120, height: 1000 });
  await page.goto("/visual/?fixture=agent&state=idle&theme=light&font-size=18&sidebar=collapsed");
  await page.locator("html[data-visual-ready]").waitFor();
  await page.evaluate(() => document.fonts.ready);
  const sources = await page.evaluate(() => {
    const png = (width: number, height: number) => {
      const canvas = document.createElement("canvas");
      canvas.width = width;
      canvas.height = height;
      const context = canvas.getContext("2d")!;
      context.fillStyle = "#3674e8";
      context.fillRect(0, 0, width, height);
      return canvas.toDataURL("image/png").split(",")[1]!;
    };
    const valid = png(600, 900);
    // The intact signature and IHDR leave only browser decoding to reject the missing body.
    return {
      valid,
      badBody: btoa(atob(valid).slice(0, 33)),
      landscape: png(900, 600),
      small: png(160, 80),
    };
  });
  const installImage = (data: string) =>
    page.evaluate(async (data) => {
      if (!location.pathname.startsWith("/visual/"))
        throw new Error("An isolated fixture is required");
      const fixturePath = "/visual/agentSessionSnapshots.ts";
      const storePath = "/src/plugins/builtin/agent/adapters/agentStore.ts";
      const projectionPath = "/src/plugins/builtin/agent/application/session/sessionSnapshot.ts";
      const fixture = (await import(fixturePath)) as typeof import("./agentSessionSnapshots");
      const { useAgentStore } = await import(storePath);
      const { projectAgentSessionSnapshot } = await import(projectionPath);
      const snapshot = structuredClone(fixture.RUNTIME_AGENT_SESSION_SNAPSHOTS.idle);
      const prompt = snapshot.items.find((item) => item.type === "userMessage");
      const answer = snapshot.items.find((item) => item.type === "agentMessage");
      if (!prompt || !answer)
        throw new Error("The fixture must contain accepted prompt and answer items");
      snapshot.items = [
        prompt,
        {
          ...answer,
          content: [
            { type: "text", text: "Before protocol image." },
            { type: "image", mime: "image/png", data },
            { type: "text", text: "After protocol image." },
          ],
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
    }, data);
  const gaps = () =>
    page.evaluate(() => {
      const before = [...document.querySelectorAll(".md p")].find(
        (node) => node.textContent === "Before protocol image.",
      )!;
      const after = [...document.querySelectorAll(".md p")].find(
        (node) => node.textContent === "After protocol image.",
      )!;
      const content = before.closest(".msg-content")!;
      const image = content.querySelector(
        'button[data-message-image-preview-trigger], [role="img"]',
      )!;
      const bounds = image.getBoundingClientRect();
      return {
        before: bounds.top - before.getBoundingClientRect().bottom,
        after: after.getBoundingClientRect().top - bounds.bottom,
      };
    });

  await installImage(sources.valid);
  await page.getByText("After protocol image.", { exact: true }).waitFor();
  const preview = page.getByRole("button", { name: "View attached image", exact: true });
  const image = preview.locator("img");
  await image.evaluate((image: HTMLImageElement) => image.decode());
  await expect(image).toHaveAttribute("width", "600");
  await expect(image).toHaveAttribute("height", "900");
  const imageBounds = () =>
    image.evaluate((image: HTMLImageElement) => {
      const bounds = image.getBoundingClientRect();
      const frame = image.closest("button")!.getBoundingClientRect();
      const content = image.closest(".msg-content")!;
      return {
        width: bounds.width,
        height: bounds.height,
        naturalWidth: image.naturalWidth,
        naturalHeight: image.naturalHeight,
        naturalRatio: image.naturalWidth / image.naturalHeight,
        frame: { width: frame.width, height: frame.height, left: frame.left, top: frame.top },
        image: { width: bounds.width, height: bounds.height, left: bounds.left, top: bounds.top },
        contentWidth: content.clientWidth,
        contentOverflow: content.scrollWidth - content.clientWidth,
        pageOverflow: document.documentElement.scrollWidth - innerWidth,
      };
    });
  const portrait = await imageBounds();
  expect(portrait.width).toBeCloseTo(portrait.height * portrait.naturalRatio, 1);
  expect(portrait.frame).toEqual(portrait.image);
  expect(await gaps()).toEqual({ before: 20, after: 24 });

  await installImage(sources.badBody);
  const unavailable = page.getByRole("img", { name: "Image unavailable", exact: true });
  await expect(unavailable).toBeVisible();
  await expect(preview).toHaveCount(0);
  expect(await gaps()).toEqual({ before: 20, after: 24 });

  for (const [viewportWidth, data] of [
    [1120, sources.landscape],
    [1120, sources.small],
    [360, sources.landscape],
  ] as const) {
    await page.setViewportSize({ width: viewportWidth, height: 1000 });
    await installImage(data);
    await image.evaluate((image: HTMLImageElement) => image.decode());
    const bounds = await imageBounds();
    expect(bounds.frame).toEqual(bounds.image);
    expect(bounds.width).toBeCloseTo(bounds.height * bounds.naturalRatio, 1);
    expect(bounds.width).toBeLessThanOrEqual(bounds.naturalWidth);
    expect(bounds.height).toBeLessThanOrEqual(bounds.naturalHeight);
    expect(bounds.width).toBeLessThanOrEqual(bounds.contentWidth);
    expect(bounds.contentOverflow).toBe(0);
    expect(bounds.pageOverflow).toBe(0);
  }
});
