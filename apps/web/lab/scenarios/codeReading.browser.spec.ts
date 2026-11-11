import { CODE_READING_SOURCE } from "./agentSessionSnapshots";
import { expect, test } from "./test";
import { expectLabReady } from "../readiness";
import { en } from "@/lib/i18n/locales/en";
import zh from "../../../../plugins/locale-zh/package/io.github.tangerg.flame/messages.json" with { type: "json" };
for (const [theme, width, fontSize, accent] of [
  ["light", 1440, 14, ""],
  ["dark", 1120, 18, "&accent=%238b5cf6"],
] as const) {
  test(`long transcript code keeps a bounded, stable reading region and shares Dock wrap in ${theme}`, async ({
    page,
  }, testInfo) => {
    let requested = false;
    let release!: () => void;
    const highlight = new Promise<void>((resolve) => (release = resolve));
    await page.route("**/node_modules/.vite/deps/shiki_core.js*", async (route) => {
      requested = true;
      await highlight;
      await route.continue();
    });
    await page.setViewportSize({ width, height: 900 });
    try {
      await page.goto(
        `/lab/fixture.html?fixture=workspace&state=dock-trajectory&agent-state=code-reading&theme=${theme}&font-size=${fontSize}${accent}`,
      );
      await expectLabReady(page);
      const frame = page.locator('[data-markdown-copy="code-block"]');
      const source = frame.getByRole("region", { name: "Code content", exact: true });
      await expect.poll(() => requested).toBe(true);
      await expect(source).toHaveText(CODE_READING_SOURCE);
      await expect(source.locator("code .line")).toHaveCount(0);
      await source.evaluate((node) => node.scrollIntoView({ block: "center" }));
      await source.focus();
      const before = await source.evaluate((node) => ({
        width: node.clientWidth,
        height: node.clientHeight,
        contentHeight: node.scrollHeight,
        limit: Math.min(
          28 * parseFloat(getComputedStyle(document.documentElement).fontSize),
          innerHeight * 0.6,
        ),
      }));
      expect(before.contentHeight).toBeGreaterThan(before.height);
      expect(before.height).toBeLessThanOrEqual(before.limit);
      await expect(frame.getByRole("button", { name: "Copy code" })).toBeInViewport();
      release();
      await expect(source.locator("code .line")).toHaveCount(48);
      await expect(source).toBeFocused();
      const after = await source.evaluate((node) => ({
        width: node.clientWidth,
        height: node.clientHeight,
      }));
      expect(after).toEqual({ width: before.width, height: before.height });
      await expect(source).toHaveText(CODE_READING_SOURCE);
      await expect(source).toBeInViewport({ ratio: 1 });
      await testInfo.attach("code-reader.png", {
        body: await page.screenshot(),
        contentType: "image/png",
      });

      const transcript = page.locator(".msg-scroll-viewport");
      const outerTop = await transcript.evaluate((node) => node.scrollTop);
      await page.keyboard.down("ArrowDown");
      try {
        await expect.poll(() => source.evaluate((node) => node.scrollTop)).toBeGreaterThan(0);
      } finally {
        await page.keyboard.up("ArrowDown");
      }
      expect(await transcript.evaluate((node) => node.scrollTop)).toBe(outerTop);

      const dock = page.locator('[data-dock-view-id="package:visual:trajectory"]');
      const record = dock.locator('[data-trajectory-record="item:item_code_reading"]');
      await record.getByRole("button").first().click();
      await record.getByRole("button", { name: "Recorded evidence", exact: true }).click();
      const evidence = record.getByRole("region", { name: "Recorded evidence", exact: true });
      await expect(evidence.locator("code .line").first()).toBeAttached();
      const wrap = frame.getByRole("button", { name: "Enable word wrap", exact: true });
      await expect(wrap).not.toHaveAttribute("aria-pressed");
      await wrap.click();
      await expect(source).toHaveAttribute("data-wrap", "true");
      await expect(evidence).toHaveAttribute("data-wrap", "true");
      const unwrap = frame.getByRole("button", { name: "Disable word wrap", exact: true });
      await expect(unwrap).not.toHaveAttribute("aria-pressed");
      await expect(unwrap).toBeFocused();
      await expect(source).toHaveText(CODE_READING_SOURCE);
      await unwrap.press("Enter");
      await expect(source).toHaveAttribute("data-wrap", "false");
      await expect(evidence).toHaveAttribute("data-wrap", "false");
      await expect(wrap).toBeFocused();
      await expect(source).toHaveText(CODE_READING_SOURCE);
      expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(0);
      const continuation = page.getByText(
        "The explanation continues with the same reading rhythm.",
        { exact: true },
      );
      await continuation.scrollIntoViewIfNeeded();
      await expect(continuation).toBeInViewport();
    } finally {
      release();
    }
  });
}

for (const browserName of ["chromium", "webkit"] as const) {
  for (const profile of [
    {
      locale: "en",
      theme: "light",
      fontSize: 14,
      motion: "full",
      accent: "#FB6E30",
      messages: en,
      evidence: "Recorded evidence",
      draft: "  Keep this exact review draft.\nRead the captured boundary.  ",
      continuation: " Continue reviewing.",
    },
    {
      locale: "zh",
      theme: "dark",
      fontSize: 18,
      motion: "off",
      accent: "#3674E8",
      messages: zh,
      evidence: "已记录的证据",
      draft: "  保留这份核对草稿。\n继续阅读原始记录。  ",
      continuation: "再核对一次。",
    },
  ] as const) {
    const reading = test.extend({
      browserName,
      reducedMotion: profile.motion === "full" ? "no-preference" : "reduce",
    });
    reading(
      `${browserName} code reading retains its settled frame and material return in ${profile.locale}`,
      async ({ page }, testInfo) => {
        const messages: Record<string, string> = profile.messages;
        const name = (key: string) => ({ name: messages[key]!, exact: true });
        const capture = async (label: string) =>
          testInfo.attach(`code-reading-${label}`, {
            body: await page.screenshot({ animations: "disabled", caret: "hide" }),
            contentType: "image/png",
          });
        await page.setViewportSize({ width: 1120, height: 755 });
        const query = new URLSearchParams({
          fixture: "workspace",
          state: "dock-trajectory",
          "agent-state": "code-reading",
          locale: profile.locale,
          theme: profile.theme,
          "font-size": String(profile.fontSize),
          motion: profile.motion,
          accent: profile.accent,
        });
        await page.goto(`/lab/fixture.html?${query}`);
        await expectLabReady(page);
        await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
        const composer = page.getByRole("textbox", name("composer.input.label"));
        await composer.fill(profile.draft);
        const input = (await composer.elementHandle())!;
        const transcript = page.locator(".msg-scroll-viewport");
        const frame = transcript.locator('[data-markdown-copy="code-block"]');
        const source = frame.getByRole("region", name("message.code.content"));
        await expect(source.locator("code .line")).toHaveCount(48);
        expect(await source.textContent()).toBe(CODE_READING_SOURCE);
        const reader = (await source.elementHandle())!;
        await frame.getByRole("button", name("message.code.copy")).focus();
        await page.keyboard.press("Tab");
        await expect(source).toBeFocused();
        const alignment = await source.evaluate((node) => ({
          source: node.querySelector("code .line")!.getBoundingClientRect().left,
          caption: node
            .closest('[data-slot="code-block"]')!
            .querySelector('[data-slot="code-caption"] > span')!
            .getBoundingClientRect().left,
        }));
        await testInfo.attach("code-reading-leading-alignment", {
          body: JSON.stringify(alignment),
          contentType: "application/json",
        });
        expect(Math.abs(alignment.source - alignment.caption)).toBeLessThan(0.5);

        const expectReading = async (label: string, focusTarget = source) => {
          const geometry = await source.evaluate(async (node, jumpLabel) => {
            const viewport = node.closest<HTMLElement>(".msg-scroll-viewport")!;
            const frames = [];
            // The first callback can precede layout delivery; keep it beside the settled samples.
            for (let index = 0; index < 16; index++) {
              await new Promise(requestAnimationFrame);
              const bounds = node.getBoundingClientRect();
              const outside = viewport.getBoundingClientRect();
              const style = getComputedStyle(viewport);
              const text = getComputedStyle(node);
              const jump = [...document.querySelectorAll<HTMLButtonElement>("button")].find(
                (button) =>
                  button.getAttribute("aria-label") === jumpLabel &&
                  button.getClientRects().length > 0 &&
                  getComputedStyle(button).visibility !== "hidden",
              );
              const action = jump?.getBoundingClientRect();
              const overlap = action
                ? Math.max(
                    0,
                    Math.min(bounds.right, action.right) - Math.max(bounds.left, action.left),
                  ) *
                  Math.max(
                    0,
                    Math.min(bounds.bottom, action.bottom) - Math.max(bounds.top, action.top),
                  )
                : 0;
              const hit = document.elementFromPoint(
                bounds.left + bounds.width / 2,
                bounds.top + bounds.height / 2,
              );
              frames.push({
                bounds: bounds.toJSON(),
                viewportScrollTop: viewport.scrollTop,
                viewportScrollRange: viewport.scrollHeight - viewport.clientHeight,
                overflowAnchor: style.overflowAnchor,
                top: outside.top + parseFloat(style.scrollPaddingTop),
                bottom: outside.bottom - parseFloat(style.scrollPaddingBottom),
                textHeight:
                  node.clientHeight - parseFloat(text.paddingTop) - parseFloat(text.paddingBottom),
                lineHeight: parseFloat(text.lineHeight),
                overlap,
                hit: hit !== null && node.contains(hit),
              });
            }
            return { delivery: frames.slice(0, 8), settled: frames.slice(8) };
          }, messages["chat.jumpToBottom"]!);
          await testInfo.attach(`code-reading-${label}-geometry`, {
            body: JSON.stringify(geometry),
            contentType: "application/json",
          });
          for (const sample of [...geometry.delivery.slice(1), ...geometry.settled]) {
            expect(sample.bounds.top).toBeGreaterThanOrEqual(sample.top - 1);
            expect(sample.bounds.bottom).toBeLessThanOrEqual(sample.bottom + 1);
            expect(sample.textHeight).toBeGreaterThanOrEqual(sample.lineHeight);
            expect(sample.overlap).toBe(0);
            expect(sample.hit).toBe(true);
          }
          expect(await reader.evaluate((node) => node.isConnected)).toBe(true);
          await expect(focusTarget).toBeFocused();
          expect(await source.evaluate((node, held) => node === held, reader)).toBe(true);
          expect(await composer.evaluate((node, held) => node === held, input)).toBe(true);
          expect(await source.textContent()).toBe(CODE_READING_SOURCE);
          await expect(composer).toHaveValue(profile.draft);
          expect(await page.evaluate(() => document.documentElement.scrollWidth - innerWidth)).toBe(
            0,
          );
        };

        const expectJumpClearance = async (label: string) => {
          await page.mouse.move(4, 4);
          const jump = page.getByRole("button", name("chat.jumpToBottom"));
          await expect(jump).toBeVisible();
          const geometry = await jump.evaluate((node) => {
            const viewport = document.querySelector<HTMLElement>(".msg-scroll-viewport")!;
            const frame = viewport.getBoundingClientRect();
            const bounds = node.getBoundingClientRect();
            const style = getComputedStyle(viewport);
            const inset = parseFloat(style.getPropertyValue("--transcript-fade"));
            const clip = {
              x: Math.ceil(frame.left + inset),
              y: Math.ceil(bounds.top) + 1,
              width: Math.floor(frame.width - inset * 2),
              height: Math.floor(bounds.height) - 2,
            };
            const y = bounds.top + bounds.height / 2;
            return {
              clip,
              background: style.backgroundColor,
              exclude: [bounds.left - clip.x - inset, bounds.right - clip.x + inset],
              historyHits: [clip.x + clip.width / 5, clip.x + (clip.width * 4) / 5].map((x) =>
                viewport.contains(document.elementFromPoint(x, y)),
              ),
              jumpHit: node.contains(document.elementFromPoint(bounds.left + bounds.width / 2, y)),
            };
          });
          const image = await page.screenshot({
            clip: geometry.clip,
            scale: "css",
            animations: "disabled",
            caret: "hide",
          });
          const paint = await page.evaluate(
            async ({ encoded, background, exclude }) => {
              const canvas = document.createElement("canvas");
              const ctx = canvas.getContext("2d", { willReadFrequently: true })!;
              canvas.width = canvas.height = 1;
              ctx.fillStyle = background;
              ctx.fillRect(0, 0, 1, 1);
              const expected = [...ctx.getImageData(0, 0, 1, 1).data];
              const image = new Image();
              image.src = `data:image/png;base64,${encoded}`;
              await image.decode();
              canvas.width = image.naturalWidth;
              canvas.height = image.naturalHeight;
              ctx.drawImage(image, 0, 0);
              const pixels = ctx.getImageData(0, 0, canvas.width, canvas.height).data;
              let samples = 0;
              let ink = 0;
              let maxDelta = 0;
              for (let y = 0; y < canvas.height; y++) {
                for (let x = 0; x < canvas.width; x++) {
                  if (x >= exclude[0]! && x <= exclude[1]!) continue;
                  const offset = (y * canvas.width + x) * 4;
                  const delta = Math.max(
                    ...expected
                      .slice(0, 3)
                      .map((value, channel) => Math.abs(pixels[offset + channel]! - value)),
                  );
                  samples++;
                  if (delta > 2) ink++;
                  maxDelta = Math.max(maxDelta, delta);
                }
              }
              return { samples, ink, maxDelta, expected };
            },
            { encoded: image.toString("base64"), ...geometry },
          );
          await testInfo.attach(`code-reading-${label}-jump-band`, {
            body: image,
            contentType: "image/png",
          });
          await testInfo.attach(`code-reading-${label}-jump-paint`, {
            body: JSON.stringify({ ...geometry, paint }),
            contentType: "application/json",
          });
          expect(paint.samples).toBeGreaterThan(0);
          expect(paint.ink).toBe(0);
          expect(geometry.historyHits).toEqual([false, false]);
          expect(geometry.jumpHit).toBe(true);
        };

        const expectSourcePerimeter = async () => {
          await page.mouse.move(4, 4);
          await expect(source).toBeFocused();
          const code = source.locator("code");
          const originalStyle = await code.getAttribute("style");
          const originalTop = await source.evaluate((node) => node.scrollTop);
          const samples = [];
          try {
            for (let phase = 0; phase < 4; phase++) {
              const geometry = await source.evaluate(
                async (node, { originalTop, phase }) => {
                  node.scrollTop =
                    originalTop + (parseFloat(getComputedStyle(node).lineHeight) * phase) / 4;
                  await new Promise(requestAnimationFrame);
                  const bounds = node.getBoundingClientRect();
                  return {
                    scrollTop: node.scrollTop,
                    clip: {
                      x: Math.ceil(bounds.left + 24),
                      y: Math.ceil(bounds.bottom) - 2,
                      width: Math.floor(bounds.right - 24) - Math.ceil(bounds.left + 24),
                      height: 2,
                    },
                  };
                },
                { originalTop, phase },
              );
              const visible = await page.screenshot({
                clip: geometry.clip,
                scale: "css",
                animations: "disabled",
                caret: "hide",
              });
              let empty: Buffer;
              try {
                await code.evaluate(async (node) => {
                  node.style.visibility = "hidden";
                  await new Promise(requestAnimationFrame);
                  await new Promise(requestAnimationFrame);
                });
                empty = await page.screenshot({
                  clip: geometry.clip,
                  scale: "css",
                  animations: "disabled",
                  caret: "hide",
                });
              } finally {
                await code.evaluate((node, style) => {
                  if (style === null) node.removeAttribute("style");
                  else node.setAttribute("style", style);
                }, originalStyle);
              }
              const paint = await page.evaluate(
                async (encoded) => {
                  const images = await Promise.all(
                    encoded.map(async (value) => {
                      const image = new Image();
                      image.src = `data:image/png;base64,${value}`;
                      await image.decode();
                      const canvas = document.createElement("canvas");
                      canvas.width = image.naturalWidth;
                      canvas.height = image.naturalHeight;
                      const ctx = canvas.getContext("2d", { willReadFrequently: true })!;
                      ctx.drawImage(image, 0, 0);
                      return ctx.getImageData(0, 0, canvas.width, canvas.height).data;
                    }),
                  );
                  let ink = 0;
                  let maxDelta = 0;
                  for (let offset = 0; offset < images[0]!.length; offset += 4) {
                    const delta = Math.max(
                      ...[0, 1, 2].map((channel) =>
                        Math.abs(images[0]![offset + channel]! - images[1]![offset + channel]!),
                      ),
                    );
                    if (delta > 2) ink++;
                    maxDelta = Math.max(maxDelta, delta);
                  }
                  return { pixels: images[0]!.length / 4, ink, maxDelta };
                },
                [visible.toString("base64"), empty.toString("base64")],
              );
              for (const [name, body] of [
                ["source", visible],
                ["empty", empty],
              ] as const)
                await testInfo.attach(`code-reading-perimeter-${phase}-${name}`, {
                  body,
                  contentType: "image/png",
                });
              samples.push({ phase, ...geometry, paint });
            }
          } finally {
            await source.evaluate((node, top) => (node.scrollTop = top), originalTop);
          }
          await testInfo.attach("code-reading-perimeter-paint", {
            body: JSON.stringify(samples),
            contentType: "application/json",
          });
          for (const sample of samples) {
            expect(sample.paint.pixels).toBeGreaterThan(0);
            expect(sample.paint.ink).toBe(0);
          }
          expect(await source.evaluate((node) => node.scrollTop)).toBe(originalTop);
          await expect(source).toBeFocused();
          expect(await source.textContent()).toBe(CODE_READING_SOURCE);
        };

        await expectReading("entry");
        // Screencast frames are what the browser presented; sampling is throttled (WebKit ≤ 25fps).
        const captureResize = async (size: { width: number; height: number }, label: string) => {
          const originals: Buffer[] = [];
          const frames: {
            timestamp: number;
            viewportWidth: number;
            viewportHeight: number;
            attachment: string;
          }[] = [];
          let requestedAt = 0;
          let resizedAt = 0;
          await page.screencast.start({
            size: { width: 1120, height: 755 },
            quality: 100,
            onFrame: ({ data, timestamp, viewportWidth, viewportHeight }) => {
              let index = originals.findIndex((original) => original.equals(data));
              if (index < 0) index = originals.push(data) - 1;
              frames.push({
                timestamp,
                viewportWidth,
                viewportHeight,
                attachment: `code-reading-${label}-frame-${index}.jpg`,
              });
            },
          });
          try {
            requestedAt = Date.now();
            await page.setViewportSize(size);
            resizedAt = Date.now();
            await expectReading(label);
          } finally {
            try {
              await page.screencast.stop();
            } finally {
              const presented = originals.slice();
              const index = frames.slice();
              for (const [position, body] of presented.entries()) {
                await testInfo.attach(`code-reading-${label}-frame-${position}.jpg`, {
                  body,
                  contentType: "image/jpeg",
                });
              }
              await testInfo.attach(`code-reading-${label}-frames`, {
                body: JSON.stringify({ requestedAt, resizedAt, frames: index }),
                contentType: "application/json",
              });
            }
          }
          expect(
            frames.some(
              (frame) =>
                frame.timestamp >= resizedAt &&
                frame.viewportWidth === size.width &&
                frame.viewportHeight === size.height,
            ),
          ).toBe(true);
        };

        for (const size of [
          { width: 560, height: 377 },
          { width: 373, height: 251 },
        ]) {
          await captureResize(size, String(size.width));
        }
        await capture("short");
        await source.press("End");
        await expect
          .poll(() =>
            source.evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop),
          )
          .toBeLessThanOrEqual(1);
        await source.hover();
        const beforeWheel = await source.evaluate((node) => node.scrollTop);
        const outerTop = await transcript.evaluate((node) => node.scrollTop);
        await page.mouse.wheel(0, -60);
        await expect
          .poll(() => source.evaluate((node) => node.scrollTop))
          .toBeLessThan(beforeWheel);
        expect(await transcript.evaluate((node) => node.scrollTop)).toBe(outerTop);
        for (const size of [
          { width: 504, height: 316 },
          { width: 373, height: 251 },
        ]) {
          await captureResize(size, `wheel-${size.width}`);
        }
        await capture("wheel-reflow");
        await expectSourcePerimeter();
        const readingPosition = await source.evaluate((node) => node.scrollTop);

        await page.getByRole("button", name("dock.action.openFull")).click();
        const material = page.locator('[data-slot="workspace-view-body"]:visible');
        await expect(material).toBeFocused();
        const record = material.locator('[data-trajectory-record="item:item_code_reading"]');
        const disclosure = record.getByRole("button").first();
        await disclosure.focus();
        await page.keyboard.press("Enter");
        await page.keyboard.press("Tab");
        const evidence = record.getByRole("button", { name: profile.evidence, exact: true });
        await expect(evidence).toBeFocused();
        await page.keyboard.press("Enter");
        const raw = record.getByRole("region", { name: profile.evidence, exact: true });
        for (const stop of [
          raw.locator("..").getByRole("button", name("message.code.wrap.enable")),
          raw.locator("..").getByRole("button", name("message.code.copy")),
          raw,
        ]) {
          await page.keyboard.press("Tab");
          await expect(stop).toBeFocused();
        }
        await expect(raw).toBeInViewport({ ratio: 1 });
        expect(
          await raw.evaluate((node) => {
            const bounds = node.getBoundingClientRect();
            return node.contains(
              document.elementFromPoint(
                bounds.left + bounds.width / 2,
                bounds.top + bounds.height / 2,
              ),
            );
          }),
        ).toBe(true);
        const captured: unknown = JSON.parse(await raw.innerText());
        expect(captured).toMatchObject({ type: "item", item: { id: "item_code_reading" } });
        expect(captured).toHaveProperty(
          "item.content.0.text",
          expect.stringContaining(CODE_READING_SOURCE),
        );
        await capture("recorded-evidence");
        await page.setViewportSize({ width: 504, height: 316 });
        await expect(raw).toBeInViewport({ ratio: 1 });
        await page.setViewportSize({ width: 373, height: 251 });
        await expect(raw).toBeInViewport({ ratio: 1 });
        await expect(raw).toBeFocused();
        await raw.hover();
        await page.getByRole("button", name("common.close")).hover();
        const tooltip = page.getByRole("tooltip");
        await expect(tooltip).toHaveText(messages["common.close"]!);
        await capture("material-tooltip");
        await page.keyboard.press("Escape");
        await expect(tooltip).toHaveCount(0);
        await expect(raw).toBeFocused();
        await expect(material).toBeVisible();
        expect(await input.evaluate((node) => (node as HTMLTextAreaElement).value)).toBe(
          profile.draft,
        );
        await page.keyboard.press("Escape");
        await expect(page.getByRole("region", name("shell.region.conversation"))).toBeFocused();
        expect(
          await source.evaluate((node, held) => node === held && node.isConnected, reader),
        ).toBe(true);
        expect(await composer.evaluate((node, held) => node === held, input)).toBe(true);
        expect(await source.textContent()).toBe(CODE_READING_SOURCE);
        expect(await source.evaluate((node) => node.scrollTop)).toBeCloseTo(readingPosition, 0);
        await expect(composer).toHaveValue(profile.draft);
        await capture("returned-reader");
        await expectReading(
          "returned-reader",
          page.getByRole("region", name("shell.region.conversation")),
        );
        await expectJumpClearance("returned-reader");
        const jump = page.getByRole("button", name("chat.jumpToBottom"));
        await jump.click();
        await expect(jump).toBeHidden();
        await expect
          .poll(() =>
            transcript.evaluate((node) => node.scrollHeight - node.clientHeight - node.scrollTop),
          )
          .toBeLessThanOrEqual(1);
        expect(await transcript.evaluate((node) => node.contains(document.activeElement))).toBe(
          true,
        );
        await capture("jumped-reader");
        const explanation = transcript.getByText(
          "The explanation continues with the same reading rhythm.",
          { exact: true },
        );
        await explanation.evaluate((node) => node.scrollIntoView({ block: "start" }));
        const lines = await explanation.evaluate(async (node) => {
          const viewport = node.closest<HTMLElement>(".msg-scroll-viewport")!;
          const samples = [];
          for (let index = 0; index < 8; index++) {
            await new Promise(requestAnimationFrame);
            const frame = viewport.getBoundingClientRect();
            const style = getComputedStyle(viewport);
            const range = document.createRange();
            range.selectNodeContents(node);
            samples.push({
              top: frame.top + parseFloat(style.scrollPaddingTop),
              bottom: frame.bottom - parseFloat(style.scrollPaddingBottom),
              lines: [...range.getClientRects()].map((line) => ({
                top: line.top,
                bottom: line.bottom,
                hit: node.contains(
                  document.elementFromPoint(line.left + line.width / 2, line.top + line.height / 2),
                ),
              })),
            });
          }
          return samples;
        });
        await testInfo.attach("code-reading-explanation-lines", {
          body: JSON.stringify(lines),
          contentType: "application/json",
        });
        await capture("explanation");
        for (const sample of lines) {
          expect(sample.lines.length).toBeGreaterThan(0);
          for (const line of sample.lines) {
            expect(line.top).toBeGreaterThanOrEqual(sample.top - 1);
            expect(line.bottom).toBeLessThanOrEqual(sample.bottom + 1);
            expect(line.hit).toBe(true);
          }
        }
        await transcript.evaluate((node) => node.scrollTo(0, node.scrollHeight));
        const footer = [];
        for (const action of [
          transcript.getByRole("button", name("msgActions.copy")).last(),
          transcript.getByRole("button", name("msgActions.regenerate")),
        ]) {
          footer.push(
            await action.evaluate(async (node) => {
              await new Promise(requestAnimationFrame);
              const viewport = node.closest<HTMLElement>(".msg-scroll-viewport")!;
              const frame = viewport.getBoundingClientRect();
              const style = getComputedStyle(viewport);
              const box = node.getBoundingClientRect();
              return {
                label: node.getAttribute("aria-label"),
                top: frame.top + parseFloat(style.scrollPaddingTop),
                bottom: frame.bottom - parseFloat(style.scrollPaddingBottom),
                bounds: { top: box.top, bottom: box.bottom },
                hit: node.contains(
                  document.elementFromPoint(box.left + box.width / 2, box.top + box.height / 2),
                ),
              };
            }),
          );
        }
        await testInfo.attach("code-reading-end-actions-geometry", {
          body: JSON.stringify(footer),
          contentType: "application/json",
        });
        await capture("end-actions");
        for (const action of footer) {
          expect(action.bounds.top).toBeGreaterThanOrEqual(action.top - 1);
          expect(action.bounds.bottom).toBeLessThanOrEqual(action.bottom + 1);
          expect(action.hit).toBe(true);
        }
        expect(await source.evaluate((node) => node.scrollTop)).toBeCloseTo(readingPosition, 0);
        await expect(composer).toHaveValue(profile.draft);
        await composer.focus();
        await composer.press(process.platform === "darwin" ? "Meta+ArrowDown" : "Control+End");
        expect(
          await composer.evaluate((node) => {
            const input = node as HTMLTextAreaElement;
            return [input.selectionStart, input.selectionEnd];
          }),
        ).toEqual([profile.draft.length, profile.draft.length]);
        await page.keyboard.type(profile.continuation);
        await expect(composer).toHaveValue(profile.draft + profile.continuation);
        await capture("returned-draft");
      },
    );
    reading(
      `${browserName} nested evidence keeps its focused reader after inner-wheel reflow in ${profile.locale}`,
      async ({ page, browserName: actualBrowserName }, testInfo) => {
        expect(actualBrowserName).toBe(browserName);
        const messages: Record<string, string> = profile.messages;
        const name = (key: string) => ({ name: messages[key]!, exact: true });
        await page.setViewportSize({ width: 373, height: 243 });
        const query = new URLSearchParams({
          fixture: "workspace",
          state: "dock-trajectory",
          "agent-state": "code-reading",
          locale: profile.locale,
          theme: profile.theme,
          "font-size": String(profile.fontSize),
          motion: profile.motion,
          accent: profile.accent,
          "custom-bg": profile.theme === "dark" ? "#505967" : "#afa996",
          "custom-fg": profile.theme === "dark" ? "#f0f4fb" : "#111111",
        });
        await page.goto(`/lab/fixture.html?${query}`);
        await expectLabReady(page);
        await page.getByRole("button", { name: "Collapse the workspace fixture sidebar" }).click();
        const composer = page.getByRole("textbox", name("composer.input.label"));
        const draft = [profile.draft, profile.draft, profile.draft].join("\n");
        await composer.fill(draft);
        const input = (await composer.elementHandle())!;
        await page.getByRole("button", name("dock.action.openFull")).click();
        const material = page.locator('[data-slot="workspace-view-body"]:visible');
        await material.getByRole("searchbox").fill("item_code_reading");
        const record = material.locator('[data-trajectory-record="item:item_code_reading"]');
        await record.getByRole("button").first().focus();
        await page.keyboard.press("Enter");
        await page.keyboard.press("Tab");
        const evidence = record.getByRole("button", { name: profile.evidence, exact: true });
        await expect(evidence).toBeFocused();
        await page.keyboard.press("Enter");
        const raw = record.getByRole("region", { name: profile.evidence, exact: true });
        const original = await raw.innerText();
        expect(JSON.parse(original)).toHaveProperty(
          "item.content.0.text",
          expect.stringContaining(CODE_READING_SOURCE),
        );
        await page.keyboard.press("Tab");
        await expect(
          raw.locator("..").getByRole("button", name("message.code.wrap.enable")),
        ).toBeFocused();
        await page.keyboard.press("Enter");
        await expect(raw).toHaveAttribute("data-wrap", "true");
        await page.keyboard.press("Tab");
        await page.keyboard.press("Tab");
        await expect(raw).toBeFocused();
        const reader = (await raw.elementHandle())!;
        const samples: unknown[] = [];
        const expectRevealed = async (label: string) => {
          await expect
            .poll(async () => {
              const frames = await raw.evaluate(async (node) => {
                const result = [];
                for (let index = 0; index < 8; index++) {
                  await new Promise(requestAnimationFrame);
                  const bounds = node.getBoundingClientRect();
                  let top = 0;
                  let bottom = innerHeight;
                  for (let parent = node.parentElement; parent; parent = parent.parentElement) {
                    if (getComputedStyle(parent).overflowY === "visible") continue;
                    const clip = parent.getBoundingClientRect();
                    top = Math.max(top, clip.top);
                    bottom = Math.min(bottom, clip.bottom);
                  }
                  const style = getComputedStyle(node);
                  const textHeight =
                    node.clientHeight -
                    parseFloat(style.paddingTop) -
                    parseFloat(style.paddingBottom);
                  const inset = Math.min(3, bounds.width / 4, bounds.height / 4);
                  const hits = [
                    [bounds.left + inset, bounds.top + inset],
                    [bounds.right - inset, bounds.top + inset],
                    [bounds.left + bounds.width / 2, bounds.top + bounds.height / 2],
                    [bounds.left + inset, bounds.bottom - inset],
                    [bounds.right - inset, bounds.bottom - inset],
                  ].map(([x, y]) => node.contains(document.elementFromPoint(x!, y!)));
                  result.push({
                    bounds: bounds.toJSON(),
                    top,
                    bottom,
                    textHeight,
                    lineHeight: parseFloat(style.lineHeight),
                    hits,
                  });
                }
                return result;
              });
              samples.push({ label, frames });
              return frames.every(
                (frame) =>
                  frame.bounds.top >= frame.top - 0.5 &&
                  frame.bounds.bottom <= frame.bottom + 0.5 &&
                  frame.textHeight >= frame.lineHeight &&
                  frame.hits.every(Boolean),
              );
            })
            .toBe(true);
          await expect(raw).toBeFocused();
          expect(await raw.evaluate((node, held) => node === held, reader)).toBe(true);
          expect(await raw.innerText()).toBe(original);
          await testInfo.attach(`nested-evidence-${label}.png`, {
            body: await page.screenshot({ animations: "disabled", caret: "hide" }),
            contentType: "image/png",
          });
        };
        try {
          await expectRevealed("entry");
          await raw.hover();
          const outerTop = await material.evaluate((node) => node.scrollTop);
          const innerTop = await raw.evaluate((node) => node.scrollTop);
          await page.mouse.wheel(0, 40);
          await expect.poll(() => raw.evaluate((node) => node.scrollTop)).toBeGreaterThan(innerTop);
          await expectRevealed("inner-wheel");
          expect(await material.evaluate((node) => node.scrollTop)).toBe(outerTop);
          for (const size of [
            { width: 504, height: 316 },
            { width: 373, height: 243 },
          ]) {
            await page.setViewportSize(size);
            await expectRevealed(`reflow-${size.width}`);
          }
          await page.keyboard.press("Escape");
          await expect(page.getByRole("region", name("shell.region.conversation"))).toBeFocused();
          await expect(composer).toHaveValue(draft);
          expect(await composer.evaluate((node, held) => node === held, input)).toBe(true);
        } finally {
          await testInfo.attach("nested-evidence-wheel-geometry", {
            body: JSON.stringify(samples),
            contentType: "application/json",
          });
          await testInfo.attach("nested-evidence-wheel.png", {
            body: await page.screenshot({ animations: "disabled", caret: "hide" }),
            contentType: "image/png",
          });
        }
      },
    );
  }
}
