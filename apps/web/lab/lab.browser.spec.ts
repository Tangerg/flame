import { expect, test, type Page } from "./scenarios/test";
import { OBSERVATION_CHANNEL, OBSERVATION_VERSION } from "./observations";
import { LAB_INSTANCE_QUERY } from "./selection";

const LAB_PATH = "/lab/";
const DESKTOP_VIEWPORT = { width: 1440, height: 1000 };

async function inspect(
  page: Page,
  story: string,
  variant: string,
  options: Record<string, string> = {},
) {
  const query = new URLSearchParams({
    story,
    variant,
    locale: "en",
    theme: "light",
    mode: "inspect",
    ...options,
  });
  await page.goto(`${LAB_PATH}?${query}`);
  await expect(page.getByRole("region", { name: "Preview canvas", exact: true })).toBeVisible();
  await expect(page.locator("[data-preview-status]")).toHaveAttribute(
    "data-preview-status",
    "ready",
    { timeout: 30_000 },
  );
}

function preview(page: Page, title: string, theme: "light" | "dark" = "light") {
  return page.getByTitle(`Component preview · ${title} · ${theme}`, { exact: true });
}

test.beforeEach(async ({ page }) => {
  await page.setViewportSize(DESKTOP_VIEWPORT);
});

test("browses both catalogs and filters components by their source", async ({ page }) => {
  await page.goto(`${LAB_PATH}?locale=en`);
  await expect(page.getByRole("heading", { level: 1, name: "Foundations" })).toBeVisible();
  const sidebar = page.getByRole("complementary", { name: "Component catalog" });
  await sidebar.getByRole("button", { name: /^Product components/ }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Product components" })).toBeVisible();
  const search = sidebar.getByRole("searchbox", { name: "Find a component, scenario or source…" });
  await search.fill("system-message.tsx");
  const navigation = sidebar.getByRole("navigation", { name: "Component catalog" });
  await expect(navigation.getByRole("button")).toHaveCount(1);
  await navigation.getByRole("button", { name: /^Product feedback/ }).click();
  await expect(page.getByRole("heading", { level: 1, name: "Product feedback" })).toBeVisible();
  await expect(
    preview(page, "Product feedback")
      .contentFrame()
      .getByRole("main", { name: "Product feedback" }),
  ).toBeVisible();

  await sidebar.getByRole("button", { name: /^Foundations/ }).click();
  await expect(search).toHaveValue("");
  await search.fill("no-such-component-65742");
  await expect(page.getByRole("heading", { name: "No matching components" })).toBeVisible();
  await sidebar.getByRole("button", { name: "Clear search", exact: true }).click();
  await expect(search).toHaveValue("");
  await expect(navigation.getByRole("button", { name: /^Button/ })).toBeVisible();
});

test("keeps the iframe and its draft through camera and viewport changes", async ({ page }) => {
  await inspect(page, "text-field", "styles");
  await expect(page.getByRole("heading", { level: 1, name: "TextField" })).toBeVisible();
  const frameElement = preview(page, "TextField");
  const frame = frameElement.contentFrame();
  const field = frame.getByRole("textbox", { name: "boxed", exact: true });
  await field.fill("Keep this local draft through camera changes");
  const originalSource = await frameElement.getAttribute("src");
  const originalDocument = await frame.locator("html").elementHandle();
  const camera = page.getByRole("toolbar", { name: "Preview canvas" });
  await camera.getByRole("button", { name: "Zoom out", exact: true }).click();
  await camera.getByRole("button", { name: "Zoom in", exact: true }).click();
  await camera.getByRole("button", { name: "Fit canvas", exact: true }).click();
  await camera.getByTitle("Center at 100%", { exact: true }).click();
  await expect(field).toHaveValue("Keep this local draft through camera changes");

  await page.getByRole("combobox", { name: "Preview viewport", exact: true }).click();
  await page.getByRole("option", { name: "Phone · 390", exact: true }).click();
  await expect(frameElement).toHaveAttribute("width", "390");
  await page.setViewportSize({ width: 1180, height: 840 });
  await expect(field).toHaveValue("Keep this local draft through camera changes");
  await expect
    .poll(async () => {
      const bounds = (await page.locator("[data-lab-pan-surface]").boundingBox())!;
      const component = (await frameElement.boundingBox())!;
      return (
        component.x >= bounds.x &&
        component.y >= bounds.y &&
        component.x + component.width <= bounds.x + bounds.width &&
        component.y + component.height <= bounds.y + bounds.height
      );
    })
    .toBe(true);
  await expect(frameElement).toHaveAttribute("src", originalSource!);
  expect(await originalDocument!.evaluate((element) => element === document.documentElement)).toBe(
    true,
  );

  const events = page.getByRole("list", { name: "Observations" });
  await expect(events.getByText("valueChange.length", { exact: true })).toBeVisible();
  await expect(events).not.toContainText("Keep this local draft");
});

test("shows gallery components at native scale and keeps their draft through responsive reflow", async ({
  page,
}) => {
  await page.goto(`${LAB_PATH}?locale=en`);
  await expect(page.getByRole("complementary", { name: "Preview inputs" })).toHaveCount(0);
  const frameElement = preview(page, "TextField");
  const field = frameElement.contentFrame().getByRole("textbox", { name: "boxed", exact: true });
  await field.fill("Retain gallery draft");
  const source = await frameElement.getAttribute("src");
  await page.setViewportSize({ width: 390, height: 844 });
  await expect(field).toHaveValue("Retain gallery draft");
  await expect(frameElement).toHaveAttribute("src", source!);
  await expect
    .poll(async () => Math.round((await frameElement.boundingBox())!.width))
    .toBe(Number(await frameElement.getAttribute("width")));
  expect(await field.evaluate((element) => getComputedStyle(element).fontSize)).toBe("14px");
  await page.getByRole("combobox", { name: "Component catalog", exact: true }).click();
  await page.getByRole("option", { name: "TextField", exact: true }).click();
  await expect(page.getByRole("heading", { level: 1, name: "TextField" })).toBeVisible();
  await expect(page.getByRole("complementary", { name: "Preview inputs" })).toBeVisible();
});

test("shares font-size inputs with independently opened previews", async ({ page }) => {
  await inspect(page, "text-field", "styles");
  await page.getByRole("combobox", { name: "Font size", exact: true }).click();
  await page.getByRole("option", { name: "18 px", exact: true }).click();
  await expect(page).toHaveURL(/font-size=18/);
  const field = preview(page, "TextField")
    .contentFrame()
    .getByRole("textbox", { name: "boxed", exact: true });
  await expect(field).toBeVisible();
  await expect
    .poll(() => field.evaluate((element) => getComputedStyle(element).fontSize))
    .toBe("18px");
  const url = new URL(
    (await page.getByRole("link", { name: "Open preview", exact: true }).getAttribute("href"))!,
    page.url(),
  );
  expect(url.searchParams.get("font-size")).toBe("18");
  await page.goto(url.href);
  await expect(page.locator("html")).toHaveAttribute("data-lab-ready", "true");
  expect(
    await page
      .getByRole("textbox", { name: "boxed", exact: true })
      .evaluate((element) => getComputedStyle(element).fontSize),
  ).toBe("18px");
});

test("keeps navigation and controls within narrow English and Chinese layouts", async ({
  page,
}) => {
  for (const locale of ["en", "zh-CN"] as const) {
    for (const width of [320, 640, 1120]) {
      await page.setViewportSize({ width, height: 800 });
      await page.goto(`${LAB_PATH}?locale=${locale}&font-size=18&mode=inspect`);
      await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
      const geometry = await page.evaluate(() => ({
        width: document.documentElement.clientWidth,
        content: document.documentElement.scrollWidth,
      }));
      expect(geometry.content).toBeLessThanOrEqual(geometry.width);
      if (width === 320)
        await expect(
          page.getByRole("combobox", {
            name: locale === "en" ? "Component catalog" : "组件目录",
            exact: true,
          }),
        ).toBeVisible();
    }
  }
});

test("compares real light and dark previews with independent local input", async ({ page }) => {
  await inspect(page, "checkbox", "editable");
  const light = preview(page, "Checkbox").contentFrame();
  await expect(light.getByRole("checkbox", { name: "Include details" })).toBeVisible();
  await page.getByRole("switch", { name: "Compare themes", exact: true }).click();
  const dark = preview(page, "Checkbox", "dark").contentFrame();
  await expect(dark.getByRole("checkbox", { name: "Include details" })).toBeVisible();
  await expect(light.locator("html")).toHaveClass(/theme-light/);
  await expect(dark.locator("html")).toHaveClass(/theme-dark/);
  await light.getByRole("checkbox", { name: "Include details" }).click();
  await expect(light.getByRole("checkbox", { name: "Include details" })).toBeChecked();
  await expect(dark.getByRole("checkbox", { name: "Include details" })).not.toBeChecked();
});

test("keeps native popover and dialog focus under canvas scaling", async ({ page }) => {
  await inspect(page, "popover", "bottom");
  const popoverFrame = preview(page, "Popover").contentFrame();
  const trigger = popoverFrame.getByRole("button", { name: "Preview details", exact: true });
  await expect(trigger).toBeVisible();
  await page
    .getByRole("toolbar", { name: "Preview canvas" })
    .getByRole("button", { name: "Zoom out", exact: true })
    .click();
  await trigger.click();
  const popover = popoverFrame.getByRole("dialog", { name: "Preview details", exact: true });
  await expect(popover).toBeVisible();
  const popoverInput = popover.getByRole("textbox", { name: "Note", exact: true });
  await popoverInput.fill("Draft inside the popover");
  await popoverInput.press("Escape");
  await expect(popover).toBeHidden();
  await expect(trigger).toBeFocused();

  await inspect(page, "modal-dialog", "md");
  const modalFrame = preview(page, "ModalDialog").contentFrame();
  const opener = modalFrame.getByRole("button", { name: "Open dialog", exact: true });
  await expect(opener).toBeVisible();
  await page
    .getByRole("toolbar", { name: "Preview canvas" })
    .getByRole("button", { name: "Zoom out", exact: true })
    .click();
  await opener.click();
  const dialog = modalFrame.getByRole("dialog", { name: "Local component preview", exact: true });
  await dialog.getByRole("textbox", { name: "Note", exact: true }).fill("Retained dialog draft");
  await dialog.getByRole("button", { name: "Cancel", exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(opener).toBeFocused();
  await opener.click();
  await expect(dialog.getByRole("textbox", { name: "Note", exact: true })).toHaveValue(
    "Retained dialog draft",
  );
  await dialog.getByRole("button", { name: "Save draft", exact: true }).click();
  await expect(dialog).toBeHidden();
  await expect(opener).toBeFocused();
  await expect(modalFrame.getByRole("status")).toHaveText("The local draft was saved.");
  await expect(page.getByRole("list", { name: "Observations" })).not.toContainText(
    "Retained dialog draft",
  );
});

test("records real activations and clears observations without resetting the example", async ({
  page,
}) => {
  await inspect(page, "empty-state", "compact");
  const frame = preview(page, "EmptyState").contentFrame();
  await frame.getByRole("button", { name: "Add example", exact: true }).click();
  const events = page.getByRole("list", { name: "Observations" });
  await expect(events.getByText("click: Add example", { exact: true })).toBeVisible();
  await expect(events.getByText("create", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Clear events", exact: true }).click();
  await expect(events.getByRole("listitem")).toHaveCount(0);
  await expect(frame.getByRole("status")).toHaveText("A local example was added.");
  await frame.getByRole("button", { name: "Reset example", exact: true }).click();
  await expect(events.getByText("reset", { exact: true })).toBeVisible();
  await expect(frame.getByRole("button", { name: "Add example", exact: true })).toBeVisible();
});

test("rejects invalid deep links explicitly and offers a working recovery link", async ({
  page,
}) => {
  await page.goto(`${LAB_PATH}?story=checkbox&variant=not-a-variant&locale=en`);
  await expect(
    page.getByRole("heading", { name: /^(此预览链接无效|Invalid preview link)$/ }),
  ).toBeVisible();
  await expect(
    page.getByText("Unknown variant: checkbox/not-a-variant", { exact: true }),
  ).toBeVisible();
  await expect(page.locator("iframe")).toHaveCount(0);
  await page.getByRole("link", { name: /^(打开组件实验室|Open component lab)$/ }).click();
  await expect(
    page.getByRole("heading", { level: 1, name: "基础组件", exact: true }),
  ).toBeVisible();
});

test("mounts a complete product fixture without external or Runtime mutation requests", async ({
  page,
  baseURL,
}) => {
  const origin = new URL(baseURL!).origin;
  const unexpected: string[] = [];
  page.on("request", (request) => {
    const url = new URL(request.url());
    if (!url.protocol.startsWith("http")) return;
    if (url.origin !== origin || (request.method() !== "GET" && request.method() !== "HEAD")) {
      unexpected.push(`${request.method()} ${url.origin}${url.pathname}`);
    }
  });
  await inspect(page, "business-shell-scenarios", "populated", { panel: "business" });
  const frame = preview(page, "Work index scenarios").contentFrame();
  await expect(frame.locator("html[data-visual-ready]")).toBeVisible({ timeout: 30_000 });
  await expect(
    frame.getByRole("heading", { name: "A compact index for active work.", exact: true }),
  ).toBeVisible();
  await expect(frame.getByTestId("requested-work-index-state")).toHaveText("populated");
  expect(unexpected).toEqual([]);
});

test("persists camera gestures across reload without remounting previews for fit or centering", async ({
  page,
}) => {
  await inspect(page, "text-field", "styles");
  const frameElement = preview(page, "TextField");
  const frame = frameElement.contentFrame();
  const field = frame.getByRole("textbox", { name: "boxed", exact: true });
  const draft = "Keep the draft while moving the camera";
  await field.fill(draft);
  const originalSource = await frameElement.getAttribute("src");
  const originalDocument = await frame.locator("html").elementHandle();
  const toolbar = page.getByRole("toolbar", { name: "Preview canvas" });
  const percentage = toolbar.getByTitle("Center at 100%", { exact: true });
  await toolbar.getByRole("button", { name: "Zoom out", exact: true }).click();
  await toolbar.getByRole("button", { name: "Fit canvas", exact: true }).click();
  await percentage.click();
  await expect(percentage).toHaveText("100%");
  await expect(field).toHaveValue(draft);
  await expect(frameElement).toHaveAttribute("src", originalSource!);
  expect(await originalDocument!.evaluate((element) => element === document.documentElement)).toBe(
    true,
  );

  const canvas = page.getByRole("region", { name: "Preview canvas", exact: true });
  const pan = canvas.getByRole("button", { name: "Preview canvas", exact: true });
  const bounds = (await pan.boundingBox())!;
  const before = (await frameElement.boundingBox())!;
  const inset = 10;
  const displacement = { x: 64, y: 40 };
  await page.mouse.move(bounds.x + inset, bounds.y + inset);
  await page.mouse.down();
  await page.mouse.move(bounds.x + inset + displacement.x, bounds.y + inset + displacement.y, {
    steps: 4,
  });
  await page.mouse.up();
  await expect
    .poll(async () => {
      const position = (await frameElement.boundingBox())!;
      return (
        Math.abs(position.x - before.x - displacement.x) < 1 &&
        Math.abs(position.y - before.y - displacement.y) < 1
      );
    })
    .toBe(true);

  const dragged = (await frameElement.boundingBox())!;
  await pan.press("ArrowRight");
  await expect.poll(async () => (await frameElement.boundingBox())!.x).toBeLessThan(dragged.x);
  await pan.press("=");
  await expect
    .poll(async () => (await frameElement.boundingBox())!.width)
    .toBeGreaterThan(dragged.width);
  await expect(field).toHaveValue(draft);
  const saved = (await frameElement.boundingBox())!;
  const savedPercentage = await percentage.textContent();

  await page.reload();
  await expect(page.locator("[data-preview-status]")).toHaveAttribute(
    "data-preview-status",
    "ready",
    { timeout: 30_000 },
  );
  await expect(percentage).toHaveText(savedPercentage!);
  await expect
    .poll(async () => {
      const restored = (await frameElement.boundingBox())!;
      return (
        Math.abs(restored.x - saved.x) < 1 &&
        Math.abs(restored.y - saved.y) < 1 &&
        Math.abs(restored.width - saved.width) < 1
      );
    })
    .toBe(true);
});

test("rejects observations with a foreign source, origin or retired instance while accepting real clicks", async ({
  page,
}) => {
  await inspect(page, "empty-state", "compact");
  const frameElement = preview(page, "EmptyState");
  const initialSource = new URL((await frameElement.getAttribute("src"))!, page.url());
  const retiredInstance = initialSource.searchParams.get(LAB_INSTANCE_QUERY);
  expect(retiredInstance).toBeTruthy();
  await page.getByRole("button", { name: "Reload example", exact: true }).click();
  await expect(frameElement).not.toHaveAttribute(
    "src",
    initialSource.pathname + initialSource.search,
  );
  await expect(page.locator("[data-preview-status]")).toHaveAttribute(
    "data-preview-status",
    "ready",
    { timeout: 30_000 },
  );
  const currentSource = new URL((await frameElement.getAttribute("src"))!, page.url());
  const currentInstance = currentSource.searchParams.get(LAB_INSTANCE_QUERY);
  expect(currentInstance).toBeTruthy();
  expect(currentInstance).not.toBe(retiredInstance);
  const events = page.getByRole("list", { name: "Observations" });
  await expect(page.getByRole("button", { name: "Clear events", exact: true })).toBeDisabled();
  await expect(events.getByRole("listitem")).toHaveCount(0);

  const validMessage = {
    channel: OBSERVATION_CHANNEL,
    version: OBSERVATION_VERSION,
    kind: "event",
    instanceId: currentInstance!,
    name: "forged.parent-source",
    detail: "must be rejected",
  };
  await page.evaluate(
    (message) => window.postMessage(message, window.location.origin),
    validMessage,
  );
  await frameElement.evaluate((element, message) => {
    const source = (element as HTMLIFrameElement).contentWindow;
    if (!source) throw new Error("preview window is unavailable");
    window.dispatchEvent(
      new MessageEvent("message", {
        source,
        origin: "https://foreign-origin.example",
        data: { ...message, name: "forged.origin" },
      }),
    );
  }, validMessage);
  const frame = frameElement.contentFrame();
  await frame.locator("html").evaluate(
    (_, message) => {
      window.parent.postMessage(message, window.location.origin);
    },
    { ...validMessage, instanceId: retiredInstance!, name: "forged.retired-instance" },
  );

  await frame.getByRole("button", { name: "Add example", exact: true }).click();
  await expect(events.getByText("create", { exact: true })).toBeVisible();
  await expect(events.getByText("click: Add example", { exact: true })).toBeVisible();
  await expect(events).not.toContainText("forged.");
  await expect(frame.getByRole("status")).toHaveText("A local example was added.");
});

test("continues native touch panning after a cancelled pinch", async ({ page, browserName }) => {
  test.skip(
    browserName !== "chromium",
    "Native multi-touch injection requires Chromium's CDP input endpoint.",
  );
  await inspect(page, "checkbox", "editable");
  const frameElement = preview(page, "Checkbox");
  const source = await frameElement.getAttribute("src");
  const canvas = page.getByRole("region", { name: "Preview canvas", exact: true });
  const surface = canvas.getByRole("button", { name: "Preview canvas", exact: true });
  const input = await page.context().newCDPSession(page);
  await input.send("Emulation.setTouchEmulationEnabled", { enabled: true, maxTouchPoints: 2 });
  try {
    const bounds = (await surface.boundingBox())!;
    const before = (await frameElement.boundingBox())!;
    await input.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [
        { id: 1, x: bounds.x + 10, y: bounds.y + 10 },
        { id: 2, x: bounds.x + 70, y: bounds.y + 10 },
      ],
    });
    await input.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [
        { id: 1, x: bounds.x + 5, y: bounds.y + 10 },
        { id: 2, x: bounds.x + 115, y: bounds.y + 10 },
      ],
    });
    await expect
      .poll(async () => (await frameElement.boundingBox())!.width)
      .toBeGreaterThan(before.width);
    await input.send("Input.dispatchTouchEvent", { type: "touchCancel", touchPoints: [] });
    const afterPinch = (await frameElement.boundingBox())!;
    const displacement = { x: 40, y: 20 };
    const start = { x: bounds.x + 15, y: bounds.y + 15 };
    await input.send("Input.dispatchTouchEvent", {
      type: "touchStart",
      touchPoints: [{ id: 3, ...start }],
    });
    await input.send("Input.dispatchTouchEvent", {
      type: "touchMove",
      touchPoints: [{ id: 3, x: start.x + displacement.x, y: start.y + displacement.y }],
    });
    await input.send("Input.dispatchTouchEvent", { type: "touchEnd", touchPoints: [] });
    await expect
      .poll(async () => {
        const moved = (await frameElement.boundingBox())!;
        return (
          Math.abs(moved.x - afterPinch.x - displacement.x) < 1 &&
          Math.abs(moved.y - afterPinch.y - displacement.y) < 1
        );
      })
      .toBe(true);
    await expect(frameElement).toHaveAttribute("src", source!);
  } finally {
    await input.send("Emulation.setTouchEmulationEnabled", { enabled: false });
    await input.detach();
  }
});
