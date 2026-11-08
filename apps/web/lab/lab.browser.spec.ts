import { expect, test, type Page } from "./scenarios/test";
import { OBSERVATION_CHANNEL, OBSERVATION_VERSION } from "./observations";
import { expectLabReady } from "./readiness";
import { LAB_INSTANCE_QUERY } from "./selection";
import { contrastRatio, WCAG_AA_TEXT } from "@/features/theme/kit/legibility";
import { catalog } from "./catalog";
import { messages } from "./messages";
import { localize } from "./story";

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

async function expectLabFailure(page: Page, cause: string) {
  const failure = await expectLabReady(page, { timeout: 1000 }).then(
    () => null,
    (error: unknown) => error,
  );
  expect(failure).toBeInstanceOf(Error);
  expect((failure as Error).message).toContain(page.url());
  expect((failure as Error).message).toContain(cause);
}

async function expectRecoveredCatalog(page: Page, heading: string, theme: "light" | "dark") {
  await expect(page.getByRole("heading", { level: 1, name: heading, exact: true })).toBeVisible();
  await expect(page.locator("html")).toHaveClass(new RegExp(`theme-${theme}`));
  const documents = page.locator("iframe");
  await expect(documents.first()).toBeVisible();
  for (const document of await documents.all()) await expectLabReady(document.contentFrame());
}

async function galleryReading(page: Page) {
  return page
    .getByRole("article")
    .first()
    .evaluate((article) => {
      const heading = article.querySelector("h2");
      if (!heading) throw new Error("gallery result heading is missing");
      for (let owner = article.parentElement; owner; owner = owner.parentElement) {
        if (!["auto", "scroll"].includes(getComputedStyle(owner).overflowY)) continue;
        const viewport = owner.getBoundingClientRect();
        const result = heading.getBoundingClientRect();
        return {
          top: owner.scrollTop,
          height: owner.clientHeight,
          contentHeight: owner.scrollHeight,
          windowTop: window.scrollY,
          firstHeadingVisible:
            result.top >= Math.max(0, viewport.top) - 1 &&
            result.bottom <= Math.min(window.innerHeight, viewport.bottom) + 1,
        };
      }
      throw new Error("gallery reading viewport is missing");
    });
}

const SIDES = ["left", "top", "right", "bottom"] as const;

async function focusPerimeter(target: ReturnType<Page["locator"]>) {
  return target.evaluate((element, sides) => {
    const style = getComputedStyle(element);
    const width = Number.parseFloat(style.outlineWidth) || 0;
    const offset = Number.parseFloat(style.outlineOffset) || 0;
    const canvas = document.createElement("canvas");
    canvas.width = 1;
    canvas.height = 1;
    const context = canvas.getContext("2d");
    if (!context) throw new Error("focus color sampling is unavailable");
    context.fillStyle = "transparent";
    context.fillStyle = style.outlineColor;
    context.fillRect(0, 0, 1, 1);
    const alpha = context.getImageData(0, 0, 1, 1).data[3]!;
    const reach = offset + width;
    const box = element.getBoundingClientRect();
    const ring = {
      left: box.left - reach,
      top: box.top - reach,
      right: box.right + reach,
      bottom: box.bottom + reach,
    };
    const visual = window.visualViewport;
    const viewportLeft = visual?.offsetLeft ?? 0;
    const viewportTop = visual?.offsetTop ?? 0;
    const boundaries = [
      {
        name: "viewport",
        left: viewportLeft,
        top: viewportTop,
        right: Math.min(
          document.documentElement.clientWidth,
          viewportLeft + (visual?.width ?? window.innerWidth),
        ),
        bottom: Math.min(
          document.documentElement.clientHeight,
          viewportTop + (visual?.height ?? window.innerHeight),
        ),
      },
    ];
    for (
      let owner = element.parentElement;
      owner && owner !== document.body && owner !== document.documentElement;
      owner = owner.parentElement
    ) {
      const clip = getComputedStyle(owner);
      if (
        clip.overflowX === "visible" &&
        clip.overflowY === "visible" &&
        !/paint|content|strict/.test(clip.contain)
      )
        continue;
      const bounds = owner.getBoundingClientRect();
      const left = bounds.left + owner.clientLeft;
      const top = bounds.top + owner.clientTop;
      boundaries.push({
        name: `${owner.tagName.toLowerCase()} ${clip.overflowX}/${clip.overflowY}`,
        left,
        top,
        right: left + owner.clientWidth,
        bottom: top + owner.clientHeight,
      });
    }
    const cuts: { boundary: string; side: string; cut: number }[] = [];
    const observed = sides.filter((side) =>
      boundaries.every((bounds) =>
        side === "left" || side === "top"
          ? box[side] >= bounds[side] - 0.25
          : box[side] <= bounds[side] + 0.25,
      ),
    );
    for (const bounds of boundaries)
      for (const side of observed) {
        const cut =
          side === "left" || side === "top" ? bounds[side] - ring[side] : ring[side] - bounds[side];
        if (cut > 0) cuts.push({ boundary: bounds.name, side, cut });
      }
    const content = Array.from(element.children)
      .map((child) => child.getBoundingClientRect())
      .filter((bounds) => bounds.width > 0 && bounds.height > 0);
    const contentInsets = {
      left: Math.min(...content.map((bounds) => bounds.left - box.left)),
      top: Math.min(...content.map((bounds) => bounds.top - box.top)),
      right: Math.min(...content.map((bounds) => box.right - bounds.right)),
      bottom: Math.min(...content.map((bounds) => box.bottom - bounds.bottom)),
    };
    return {
      focused: element.ownerDocument.activeElement === element,
      outline: { style: style.outlineStyle, width, offset, alpha },
      reach,
      box: { left: box.left, top: box.top, right: box.right, bottom: box.bottom },
      observed,
      contentInsets,
      perimeterReserved:
        content.length > 0 &&
        Object.values(contentInsets).every((distance) => distance >= Math.max(0, -offset) - 0.25),
      fullyVisible: boundaries.every(
        (bounds) =>
          ring.left >= bounds.left - 0.25 &&
          ring.top >= bounds.top - 0.25 &&
          ring.right <= bounds.right + 0.25 &&
          ring.bottom <= bounds.bottom + 0.25,
      ),
      cut: Math.max(0, ...cuts.map(({ cut }) => cut)),
      cuts,
    };
  }, SIDES);
}

function expectContainedFocus(
  ring: Awaited<ReturnType<typeof focusPerimeter>>,
  label: string,
  complete: boolean,
) {
  expect.soft(ring.focused, `${label} holds focus`).toBe(true);
  expect.soft(ring.outline.style, `${label} paints an outline`).not.toMatch(/^(none|hidden)$/);
  expect.soft(ring.outline.width, `${label} outline has width`).toBeGreaterThan(0);
  expect.soft(ring.outline.alpha, `${label} outline is opaque`).toBeGreaterThan(0);
  if (complete) expect.soft(ring.fullyVisible, `${label} perimeter is in view`).toBe(true);
  else expect.soft(ring.observed.length, `${label} perimeter is in view`).toBeGreaterThan(0);
  expect.soft(ring.cut, `${label} outline is not clipped`).toBeLessThanOrEqual(0.25);
}

async function expectFailureSurface(page: Page, theme: "light" | "dark") {
  const paint = await page.evaluate(() => {
    const surface = document.elementFromPoint(2, 2);
    const link = document.querySelector("a[href]");
    if (!surface || !link) throw new Error("failure surface or recovery link is missing");
    const canvas = document.createElement("canvas");
    canvas.width = 3;
    canvas.height = 1;
    const context = canvas.getContext("2d");
    if (!context) throw new Error("failure color sampling is unavailable");
    const style = getComputedStyle(surface);
    [style.backgroundColor, style.color, getComputedStyle(link).color].forEach((value, index) => {
      context.fillStyle = value;
      context.fillRect(index, 0, 1, 1);
    });
    const data = context.getImageData(0, 0, 3, 1).data;
    return {
      background: { r: data[0]!, g: data[1]!, b: data[2]! },
      foreground: { r: data[4]!, g: data[5]!, b: data[6]! },
      recovery: { r: data[8]!, g: data[9]!, b: data[10]! },
      alpha: [data[3], data[7], data[11]],
    };
  });
  expect(paint.alpha).toEqual([255, 255, 255]);
  const brightness = (paint.background.r + paint.background.g + paint.background.b) / 3;
  if (theme === "dark") expect(brightness).toBeLessThan(128);
  else expect(brightness).toBeGreaterThan(128);
  expect(contrastRatio(paint.foreground, paint.background)).toBeGreaterThanOrEqual(WCAG_AA_TEXT);
  expect(contrastRatio(paint.recovery, paint.background)).toBeGreaterThanOrEqual(WCAG_AA_TEXT);
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
    .poll(() =>
      frameElement.evaluate((frame) => {
        const bounds = frame.getBoundingClientRect();
        return (
          Math.round(bounds.width) === Number(frame.getAttribute("width")) &&
          bounds.left >= 0 &&
          bounds.right <= document.documentElement.clientWidth
        );
      }),
    )
    .toBe(true);
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
  await expectLabReady(page, { timeout: 5_000 });
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

for (const locale of ["en", "zh-CN"] as const) {
  for (const viewport of [
    { width: 1120, height: 360 },
    { width: 640, height: 364 },
    { width: 373, height: 243 },
    { width: 320, height: 243 },
  ]) {
    test(`gallery navigation retains reading context ${locale} ${viewport.width}`, async ({
      page,
    }, testInfo) => {
      const theme = locale === "en" ? "light" : "dark";
      const t = (key: keyof typeof messages) => localize(messages[key], locale);
      const main = page.getByRole("main");
      const sidebar = page.getByRole("complementary", { name: t("catalog"), exact: true });
      const search = sidebar.getByRole("searchbox", { name: t("search"), exact: true });
      const gallery = page.getByRole("radio", { name: t("gallery"), exact: true });
      const story = catalog.story("slider");
      const title = localize(story.title, locale);
      const variant = story.variants.at(-1)!;
      const variants = page.getByRole("combobox", { name: t("variants"), exact: true });
      const previewDocument = () =>
        page.getByTitle(`${t("frameTitle")} · ${title} · ${theme}`, { exact: true }).contentFrame();
      const mainFocused = () =>
        main.evaluate((element) => element.ownerDocument.activeElement === element);
      const capture = async (stage: string, detail: unknown) => {
        await testInfo.attach(stage + ".json", {
          body: JSON.stringify(detail, null, 2),
          contentType: "application/json",
        });
        await testInfo.attach(stage, {
          body: await page.screenshot({ animations: "disabled", caret: "hide" }),
          contentType: "image/png",
        });
      };

      await page.setViewportSize(viewport);
      await page.goto(`${LAB_PATH}?locale=${locale}&theme=${theme}&font-size=18`);
      await expectRecoveredCatalog(page, t("foundation"), theme);
      await expect(main).toHaveAccessibleName(t("foundation"));
      const next = page.getByRole("button", { name: t("next"), exact: true });
      await next.scrollIntoViewIfNeeded();
      expect(
        (await galleryReading(page)).top,
        "paging starts from a scrolled gallery",
      ).toBeGreaterThan(0);
      await next.press("Enter");
      await expectRecoveredCatalog(page, t("foundation"), theme);
      await expect(
        page.getByRole("navigation", { name: t("pagination"), exact: true }),
      ).toContainText("2 /");
      const paged = await galleryReading(page);
      const pagedFocus = await focusPerimeter(main);
      await capture("new-gallery-page", {
        reading: paged,
        mainFocused: await mainFocused(),
        focus: pagedFocus,
      });
      expect.soft(paged.top, "a new page starts at its first result").toBeLessThanOrEqual(1);
      expect.soft(paged.firstHeadingVisible, "the first new result is readable").toBe(true);
      expect.soft(await mainFocused(), "paging hands focus to the preview context").toBe(true);
      expectContainedFocus(pagedFocus, "the paged reading region", false);
      expect
        .soft(pagedFocus.perimeterReserved, "the reading region reserves its focus perimeter")
        .toBe(true);

      const opener = page.getByRole("button", { name: `${t("select")} · ${title}`, exact: true });
      await opener.scrollIntoViewIfNeeded();
      const saved = await galleryReading(page);
      expect(saved.top, "the inspected result is below the gallery start").toBeGreaterThan(0);
      await opener.press("Enter");
      await expect(page.getByRole("heading", { level: 1, name: title, exact: true })).toBeVisible();
      await expect(main).toHaveAccessibleName(title);
      expect.soft(await mainFocused(), "the retired gallery action hands focus to main").toBe(true);
      await expectLabReady(previewDocument());
      expect.soft(await mainFocused(), "preview admission retains navigation focus").toBe(true);
      await variants.scrollIntoViewIfNeeded();
      await variants.press("Enter");
      await expect(
        page.getByRole("option", {
          name: localize(story.variants[0]!.title, locale),
          exact: true,
        }),
      ).toBeFocused();
      await expect(
        page.getByRole("option", { name: localize(variant.title, locale), exact: true }),
      ).toBeVisible();
      await page.keyboard.press("End");
      await expect(
        page.getByRole("option", { name: localize(variant.title, locale), exact: true }),
      ).toBeFocused();
      await page.keyboard.press("Enter");
      await expect(variants).toContainText(localize(variant.title, locale));
      await expect(variants).toBeFocused();
      await expectLabReady(previewDocument());
      await expect(variants).toBeFocused();

      await gallery.press("Space");
      await expectRecoveredCatalog(page, t("foundation"), theme);
      const returned = await galleryReading(page);
      await capture("returned-gallery", { saved, returned, variant: variant.id });
      expect
        .soft(Math.abs(returned.top - saved.top), "return retains the gallery reading offset")
        .toBeLessThanOrEqual(1);
      await expect(gallery).toBeFocused();
      await expect(
        page.getByRole("navigation", { name: t("pagination"), exact: true }),
      ).toContainText("2 /");
      await expect(page).toHaveURL(new RegExp(`variant=${variant.id}`));
      await expectLabReady(previewDocument());

      await search.fill(story.source);
      await expect(page.getByRole("article")).toHaveCount(1);
      await expectRecoveredCatalog(page, t("foundation"), theme);
      expect
        .soft((await galleryReading(page)).top, "a new filter starts at the first result")
        .toBeLessThanOrEqual(1);
      await expect(search).toHaveValue(story.source);
      await expect(search).toBeFocused();
      await sidebar.getByRole("button", { name: t("clearSearch"), exact: true }).click();
      await expectRecoveredCatalog(page, t("foundation"), theme);
      expect
        .soft((await galleryReading(page)).top, "clearing a filter starts the new context")
        .toBeLessThanOrEqual(1);
      await expect(search).toHaveValue("");
      await expect(search).toBeFocused();

      const fieldStory = catalog.story("text-field");
      const fieldTitle = localize(fieldStory.title, locale);
      const fieldOpener = page.getByRole("button", {
        name: `${t("select")} · ${fieldTitle}`,
        exact: true,
      });
      await fieldOpener.scrollIntoViewIfNeeded();
      expect((await galleryReading(page)).top).toBeGreaterThan(0);
      await fieldOpener.press("Enter");
      await expectLabReady(
        page
          .getByTitle(`${t("frameTitle")} · ${fieldTitle} · ${theme}`, { exact: true })
          .contentFrame(),
      );
      const catalogEntries = sidebar
        .getByRole("navigation", { name: t("catalog"), exact: true, includeHidden: true })
        .getByRole("button", { includeHidden: true });
      await search.fill(fieldStory.source);
      await expect(search).toHaveValue(fieldStory.source);
      await expect(catalogEntries).toHaveCount(
        catalog.search("foundation", fieldStory.source, locale).length,
      );
      await search.fill("");
      await expect(search).toHaveValue("");
      await expect(catalogEntries).toHaveCount(catalog.search("foundation", "", locale).length);
      await gallery.press("Space");
      await expectRecoveredCatalog(page, t("foundation"), theme);
      const changedContext = await galleryReading(page);
      expect
        .soft(changedContext.top, "filter changes while inspecting retire the old return position")
        .toBeLessThanOrEqual(1);
      await expect(gallery).toBeFocused();

      const firstTitle = localize(catalog.search("foundation", "", locale)[0]!.title, locale);
      const firstAction = page
        .getByRole("article")
        .first()
        .getByRole("button", { name: `${t("select")} · ${firstTitle}`, exact: true });
      await gallery.press("Tab");
      await expect(firstAction).toBeFocused();
      const tooltip = page.getByRole("tooltip");
      await expect(tooltip).toBeVisible();
      await firstAction.press("Escape");
      await expect(tooltip).toBeHidden();
      await expect(firstAction).toBeFocused();
      await expect(main).toHaveAccessibleName(t("foundation"));
      await expect(search).toHaveValue("");
      const actionFocus = await focusPerimeter(firstAction);
      await capture("changed-gallery-context", {
        reading: changedContext,
        actionReading: await galleryReading(page),
        focus: actionFocus,
      });
      expectContainedFocus(actionFocus, "the first gallery action", true);
    });
  }
}

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

for (const copy of [
  {
    locale: "en",
    theme: "light",
    invalid: "Invalid preview link",
    renderError: "Component could not render",
    restart: "Open component lab",
    home: "Foundations",
  },
  {
    locale: "zh-CN",
    theme: "dark",
    invalid: "此预览链接无效",
    renderError: "组件无法渲染",
    restart: "打开组件实验室",
    home: "基础组件",
  },
] as const) {
  test(`rejects invalid deep links and preserves recovery language ${copy.locale}`, async ({
    page,
  }) => {
    await page.goto(
      `${LAB_PATH}?story=checkbox&variant=not-a-variant&locale=${copy.locale}&theme=${copy.theme}`,
    );
    await expect(
      page.getByText("Unknown variant: checkbox/not-a-variant", { exact: true }),
    ).toBeVisible();
    await test.info().attach("invalid-preview-link", {
      body: await page.screenshot({ animations: "disabled", caret: "hide" }),
      contentType: "image/png",
    });
    await expectFailureSurface(page, copy.theme);
    const heading = page.getByRole("heading", { level: 1, name: copy.invalid, exact: true });
    await expect(heading).toBeVisible();
    expect(
      await heading.evaluate((element) => element.closest("[lang]")?.getAttribute("lang")),
    ).toBe(copy.locale);
    await expect(page.locator("iframe")).toHaveCount(0);
    await page.getByRole("link", { name: copy.restart, exact: true }).click();
    await expectRecoveredCatalog(page, copy.home, copy.theme);
  });

  test(`independent failed preview preserves recovery language ${copy.locale}`, async ({
    page,
  }) => {
    const cause = "Unknown variant: checkbox/not-a-variant";
    await page.goto(
      `${LAB_PATH}preview.html?story=checkbox&variant=not-a-variant&locale=${copy.locale}&theme=${copy.theme}`,
    );
    await expect(page.getByText(cause, { exact: true })).toBeVisible();
    await test.info().attach("independent-preview-failure", {
      body: await page.screenshot({ animations: "disabled", caret: "hide" }),
      contentType: "image/png",
    });
    await expectFailureSurface(page, copy.theme);
    const heading = page.getByRole("heading", { level: 1, name: copy.renderError, exact: true });
    await expect(heading).toBeVisible();
    expect(
      await heading.evaluate((element) => element.closest("[lang]")?.getAttribute("lang")),
    ).toBe(copy.locale);
    await expectLabFailure(page, cause);
    await page.getByRole("link", { name: copy.restart, exact: true }).press("Enter");
    await expectRecoveredCatalog(page, copy.home, copy.theme);
  });
}

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
  await expectLabReady(frame, { timeout: 30_000 });
  await expect(
    frame.getByRole("heading", { name: "A compact index for active work.", exact: true }),
  ).toBeVisible();
  await expect(frame.getByTestId("requested-work-index-state")).toHaveText("populated");
  expect(unexpected).toEqual([]);
});

for (const locale of ["en", "zh", null] as const) {
  test(`reports a caught fixture boot failure with its document and cause ${locale ?? "default"}`, async ({
    page,
  }) => {
    const cause = `synthetic fixture boot failure: ${"deep-component-path/".repeat(18)}fixtureRuntime.tsx`;
    await page.setViewportSize({ width: 390, height: 480 });
    const fixtureModule = (url: URL) => url.pathname === "/lab/scenarios/fixtureRuntime.tsx";
    await page.route(fixtureModule, (route) =>
      route.fulfill({
        contentType: "text/javascript",
        body: `throw new Error(${JSON.stringify(cause)});`,
      }),
    );
    const theme = locale === "zh" ? "dark" : "light";
    const query = new URLSearchParams({ fixture: "shell", theme, "font-size": "18" });
    if (locale !== null) query.set("locale", locale);
    await page.goto(`/lab/fixture.html?${query}`);
    const alert = page.getByRole("alert");
    await expect(alert.getByText(cause, { exact: true })).toBeVisible();
    await test.info().attach("fixture-boot-failure", {
      body: await page.screenshot({ animations: "disabled", caret: "hide" }),
      contentType: "image/png",
    });
    await expectFailureSurface(page, theme);
    expect(
      await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth),
    ).toBeLessThanOrEqual(1);
    expect(
      await alert.evaluate((element) => element.scrollWidth - element.clientWidth),
    ).toBeLessThanOrEqual(1);
    const chinese = locale === "zh";
    const heading = alert.getByRole("heading", {
      name: chinese ? "组件无法渲染" : "Component could not render",
      exact: true,
    });
    await expect(heading).toBeVisible();
    expect(
      await heading.evaluate((element) => element.closest("[lang]")?.getAttribute("lang")),
    ).toBe(chinese ? "zh-CN" : "en");
    await expectLabFailure(page, cause);
    await page.unroute(fixtureModule);
    await alert
      .getByRole("link", {
        name: chinese ? "打开组件实验室" : "Open component lab",
        exact: true,
      })
      .press("Enter");
    await expectRecoveredCatalog(page, chinese ? "基础组件" : "Foundations", theme);
  });
}

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
