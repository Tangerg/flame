import { COMPOSITION_SAMPLES } from "./scenarios/compositionFixtureStates";
import { expect, test } from "./scenarios/test";
import { catalog } from "./catalog";
import { labCases } from "./cases";
import { readSelection } from "./selection";
import { foundationMessages } from "./foundationMessages";
import {
  foundationButtonVariants,
  foundationControlSizes,
  foundationViews,
} from "./foundationVariants";
import { expectLabReady } from "./readiness";
import { VISUAL_LONG_SKILL_DOCUMENTS } from "./scenarios/skillSnapshots";

const NARROW_WIDTH = 320;

const filter = process.env.FLAME_LAB_STORY;
if (filter) catalog.story(filter);

for (const story of catalog.stories) {
  if (filter && story.id !== filter) continue;
  for (const viewport of ["component", "phone"] as const) {
    if (viewport === "phone" && story.kind === "fixture") continue;
    test(story.id + " renders at " + viewport + " width", async ({ page }, testInfo) => {
      const chinese = testInfo.project.name === "dark-zh";
      const query = new URLSearchParams({
        locale: chinese ? "zh-CN" : "en",
        theme: chinese ? "dark" : "light",
        "font-size": chinese ? "18" : "14",
        viewport,
      });
      const cases = labCases(catalog, readSelection(query.toString(), catalog)).filter(
        (entry) => entry.storyId === story.id,
      );
      await testInfo.attach("cases.json", {
        body: JSON.stringify({ environment: Object.fromEntries(query), cases }, null, 2),
        contentType: "application/json",
      });
      for (const entry of cases) {
        await test.step(entry.variantId, async () => {
          await page.setViewportSize({ width: entry.width, height: entry.height });
          await page.goto(entry.url);
          await expectLabReady(page, {
            timeout: 30_000,
          });
          await page.evaluate(async () => {
            await document.fonts.ready;
            await Promise.all(Array.from(document.images, (image) => image.decode()));
          });
          if (entry.kind === "component") {
            const padding = await page
              .getByRole("main")
              .evaluate((element) => parseFloat(getComputedStyle(element).paddingTop));
            expect(
              padding,
              "Independent previews must load their extracted StyleX rules",
            ).toBeGreaterThanOrEqual(16);
          } else if (story.kind === "fixture") {
            await expect(page.locator("#root")).toHaveCSS("position", "fixed");
            const target = story.variants.find((variant) => variant.id === entry.variantId)!.target;
            if (["agent", "shell", "workspace", "foundation"].includes(target.fixture)) {
              await expect(page.locator("[data-sidebar]").first()).toHaveCSS("display", "flex");
            }
          }
          if (entry.storyId === "business-composition") {
            const sample = COMPOSITION_SAMPLES.find((sample) => sample.id === entry.variantId)!;
            await expect(page.locator("#root section")).toHaveCount(sample.frames);
          }
          if (entry.storyId === "business-workspace-scenarios") {
            if (entry.variantId === "dock-usage-reports") {
              await expect(
                page
                  .locator('[data-slot="workspace-view-body"]:visible')
                  .getByRole("button", { name: chinese ? /^查看报告：/ : /^View report:/ }),
              ).toHaveCount(2);
            } else if (entry.variantId === "dock-file-error") {
              await expect(
                page
                  .getByRole("region", { name: "README.md", exact: true })
                  .getByRole("button", { name: chinese ? "重试" : "Retry", exact: true }),
              ).toBeVisible();
            } else if (entry.variantId === "dock-file-unsupported") {
              await expect(
                page
                  .getByRole("region", { name: "review-image.png", exact: true })
                  .getByText(chinese ? "这里暂时无法预览图片" : "No image preview here yet", {
                    exact: true,
                  }),
              ).toBeVisible();
            } else if (entry.variantId === "dock-diagnostics-recorded") {
              const panel = page.getByRole("region", {
                name: chinese ? "客户端诊断" : "Client diagnostics",
                exact: true,
              });
              await expect(
                panel.getByRole("radio", {
                  name: chinese ? "追踪 · 3" : "Traces · 3",
                  exact: true,
                }),
              ).toBeChecked();
              await expect(
                panel.getByRole("button", { name: /workspace\.preview\.read/ }),
              ).toBeVisible();
            } else if (entry.variantId === "dock-skills-catalog") {
              const panel = page
                .locator('[data-slot="workspace-view-body"]:visible')
                .getByRole("region", { name: chinese ? "技能" : "Skills", exact: true });
              const count = VISUAL_LONG_SKILL_DOCUMENTS.length;
              await expect(
                panel.getByRole("heading", {
                  name: chinese ? `${count} 个可用技能` : `${count} available Skills`,
                  exact: true,
                }),
              ).toBeVisible();
              await expect(
                panel.getByText(VISUAL_LONG_SKILL_DOCUMENTS[0]!.name, { exact: true }),
              ).toBeVisible();
            }
          }
          const overflow = await page.evaluate(() => ({
            viewport: document.documentElement.clientWidth,
            content: document.documentElement.scrollWidth,
          }));
          expect
            .soft(overflow.content, entry.id + ": horizontal document overflow")
            .toBeLessThanOrEqual(overflow.viewport + 1);
          const initialImage = await page.screenshot({ fullPage: true, animations: "disabled" });
          await testInfo.attach(entry.id + "-" + entry.width + "px", {
            body: initialImage,
            contentType: "image/png",
          });
          const copy = foundationMessages[chinese ? "zh-CN" : "en"];
          const captions =
            entry.variantId === foundationViews.sizes.id
              ? entry.storyId === "button"
                ? foundationControlSizes
                : entry.storyId === "icon-button"
                  ? [...foundationControlSizes, "xl"]
                  : null
              : entry.storyId === "button" && entry.variantId === foundationViews.styles.id
                ? foundationButtonVariants
                : null;
          if (captions || entry.storyId === "switch") {
            const main = page.getByRole("main");
            const box = async (locator: ReturnType<typeof page.getByRole>) =>
              (await locator.boundingBox())!;
            const geometry: (
              | { width: number; rows: { label: string; x: number; top: number }[][] }
              | { width: number; label: number; control: number }
            )[] = [];
            let phoneImage: Buffer | undefined;
            for (const width of [entry.width, NARROW_WIDTH]) {
              await page.setViewportSize({ width, height: entry.height });
              if (captions) {
                const rows: { label: string; x: number; top: number }[][] = [];
                for (const label of captions) {
                  const { x, y } = await box(main.getByText(label, { exact: true }));
                  const row = rows.at(-1);
                  if (row && x > row.at(-1)!.x) row.push({ label, x, top: y });
                  else rows.push([{ label, x, top: y }]);
                }
                geometry.push({ width, rows });
              } else {
                const center = async (locator: ReturnType<typeof page.getByRole>) => {
                  const { y, height } = await box(locator);
                  return y + height / 2;
                };
                geometry.push({
                  width,
                  label: await center(main.getByText(copy.enableNotifications, { exact: true })),
                  control: await center(
                    page.getByRole("switch", { name: copy.enableNotifications, exact: true }),
                  ),
                });
              }
              if (width === NARROW_WIDTH && viewport === "phone") {
                phoneImage = await page.screenshot({ fullPage: true, animations: "disabled" });
              }
            }
            await page.setViewportSize({ width: entry.width, height: entry.height });
            await testInfo.attach(entry.id + "-alignment.json", {
              body: JSON.stringify(geometry, null, 2),
              contentType: "application/json",
            });
            if (phoneImage) {
              await testInfo.attach(entry.id + "-" + NARROW_WIDTH + "px", {
                body: phoneImage,
                contentType: "image/png",
              });
            }
            for (const measured of geometry) {
              if ("rows" in measured) {
                for (const row of measured.rows) {
                  const tops = row.map((caption) => caption.top);
                  expect
                    .soft(
                      Math.max(...tops) - Math.min(...tops),
                      `${entry.id} at ${measured.width}px: captions ${row.map((caption) => caption.label).join(", ")} share a top`,
                    )
                    .toBeLessThanOrEqual(1);
                }
              } else {
                expect
                  .soft(
                    Math.abs(measured.label - measured.control),
                    `${entry.id} at ${measured.width}px: the Switch label and control share a center`,
                  )
                  .toBeLessThanOrEqual(1);
              }
            }
          }
          if (entry.storyId === "form") {
            const name = page.getByRole("textbox", { name: copy.name, exact: true });
            const nameLabel = page.getByText(copy.name, { exact: true });
            const error = page.getByRole("alert");
            const review = page.getByRole("group", { name: copy.review, exact: true });
            const save = page.getByRole("button", { name: copy.save, exact: true });
            const status = page.getByRole("status");
            const bounds = async (locator: typeof name) => (await locator.boundingBox())!;
            const shortHeight = 240;
            const overflowViewport = async (locator: typeof name) => {
              const box = await bounds(locator);
              return Math.max(
                -box.x,
                -box.y,
                box.x + box.width - entry.width,
                box.y + box.height - shortHeight,
              );
            };
            const submitInvalid = async (attempt: string) => {
              await save.scrollIntoViewIfNeeded();
              if (viewport === "phone") {
                await expect(
                  name,
                  attempt + ": Name starts above the viewport",
                ).not.toBeInViewport();
              }
              await save.click();
              await expect(name, attempt + ": invalid Save focuses Name").toBeFocused();
              await expect(name).toHaveAttribute("aria-invalid", "true");
              await expect(name).toHaveAccessibleDescription(copy.invalid);
              await expect(error).toBeVisible();
              await expect(error).toHaveText(copy.invalid);
              await expect(status).toBeHidden();
              await expect
                .poll(() => overflowViewport(name), attempt + ": Name is inside the viewport")
                .toBeLessThanOrEqual(1);
              await expect
                .poll(
                  () => overflowViewport(nameLabel),
                  attempt + ": the Name label is inside the viewport",
                )
                .toBeLessThanOrEqual(1);
              await expect
                .poll(() => overflowViewport(error), attempt + ": the error is inside the viewport")
                .toBeLessThanOrEqual(1);
            };
            await page.setViewportSize({ width: entry.width, height: shortHeight });
            await expect(name).not.toBeFocused();
            if (entry.variantId === "editable") {
              await expect(name).not.toHaveAttribute("aria-invalid", "true");
              await expect(error).toBeHidden();
            } else {
              await expect(name).toHaveAttribute("aria-invalid", "true");
            }
            await submitInvalid("first Save");
            await testInfo.attach(entry.id + "-invalid-" + entry.width + "px", {
              body: await page.screenshot({ fullPage: true, animations: "disabled" }),
              contentType: "image/png",
            });
            await testInfo.attach(entry.id + "-invalid-viewport-" + entry.width + "px", {
              body: await page.screenshot({ animations: "disabled" }),
              contentType: "image/png",
            });
            const field = await bounds(name);
            const message = await bounds(error);
            const group = await bounds(review);
            expect(message.y, "The error follows the Name input").toBeGreaterThanOrEqual(
              field.y + field.height - 1,
            );
            expect(Math.abs(message.x - field.x), "The error aligns with Name").toBeLessThanOrEqual(
              1,
            );
            expect(
              message.x + message.width,
              "The error stays within the Name width",
            ).toBeLessThanOrEqual(field.x + field.width + 1);
            if (group.y >= field.y + field.height - 1) {
              expect(
                message.y + message.height,
                "A stacked error precedes Review",
              ).toBeLessThanOrEqual(group.y + 1);
            } else {
              const checkbox = await bounds(
                page.getByRole("checkbox", { name: copy.accept, exact: true }),
              );
              expect(checkbox.y, "Review stays beside the Name input").toBeGreaterThanOrEqual(
                field.y - 1,
              );
              expect(
                checkbox.y + checkbox.height,
                "Review stays within the Name input row",
              ).toBeLessThanOrEqual(field.y + field.height + 1);
              expect(
                message.x + message.width,
                "The error stays out of the Review column",
              ).toBeLessThanOrEqual(group.x + 1);
            }
            const draft = chinese ? "  草稿 一  " : "  Draft one  ";
            await name.fill(draft);
            await expect(name).not.toHaveAttribute("aria-invalid", "true");
            await expect(name).toHaveAccessibleDescription("");
            await expect(error).toBeHidden();
            await save.click();
            await expect(name).toHaveValue(draft);
            await expect(status).toHaveText(copy.saved);
            await expect(save, "A successful Save keeps focus").toBeFocused();

            await name.fill("   ");
            await submitInvalid("whitespace Save");
            await expect(name).toHaveValue("   ");
            await page.setViewportSize({ width: entry.width, height: entry.height });
          }
          if (entry.storyId === "business-composition") {
            const payload = page.getByText("Consumer payload", { exact: true });
            await payload.scrollIntoViewIfNeeded();
            await expect(payload).toBeVisible();
            await testInfo.attach(entry.id + "-payload", {
              body: await page.screenshot({ fullPage: true, animations: "disabled" }),
              contentType: "image/png",
            });
          }
          if (entry.storyId === "business-reasoning") {
            const toggle = page.getByRole("button");
            await expect(toggle).toHaveAttribute("aria-expanded", "false");
            await toggle.click();
            await expect(toggle).toHaveAttribute("aria-expanded", "true");
            const reader = page.locator('[data-slot="reasoning-scroller"]');
            await expect(reader.locator(".md p").first()).toBeVisible();
            await expect
              .poll(() =>
                reader.evaluate((element) => {
                  let opacity = 1;
                  for (let node: Element | null = element; node; node = node.parentElement) {
                    opacity *= Number(getComputedStyle(node).opacity);
                  }
                  return opacity;
                }),
              )
              .toBe(1);
            const openedImage = await page.screenshot({ fullPage: true, animations: "disabled" });
            expect(openedImage.equals(initialImage)).toBe(false);
            await testInfo.attach(entry.id + "-open-" + entry.width + "px", {
              body: openedImage,
              contentType: "image/png",
            });
            await toggle.click();
            await expect(toggle).toHaveAttribute("aria-expanded", "false");
            await expect(reader).not.toBeInViewport();
            await reader.focus();
            await expect(toggle).toBeFocused();
          }
          if (entry.storyId === "popover" || entry.storyId === "modal-dialog") {
            await page
              .getByRole("button", {
                name: entry.storyId === "popover" ? copy.details : copy.openDialog,
                exact: true,
              })
              .click();
            const role = entry.variantId === "confirmation" ? "alertdialog" : "dialog";
            const overlay = page.getByRole(role);
            await expect(overlay).toBeVisible();
            await expect(overlay).toBeInViewport();
            await expect
              .poll(() =>
                overlay.evaluate((element) => {
                  let opacity = 1;
                  for (let node: Element | null = element; node; node = node.parentElement) {
                    opacity *= Number(getComputedStyle(node).opacity);
                  }
                  return opacity;
                }),
              )
              .toBe(1);
            const openedImage = await page.screenshot({ fullPage: true, animations: "disabled" });
            expect(
              openedImage.equals(initialImage),
              "Opening the overlay must change its capture",
            ).toBe(false);
            await testInfo.attach(entry.id + "-open-" + entry.width + "px", {
              body: openedImage,
              contentType: "image/png",
            });
          }
        });
      }
    });
  }
}
