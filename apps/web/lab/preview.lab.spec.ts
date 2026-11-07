import { COMPOSITION_SAMPLES } from "./scenarios/compositionFixtureStates";
import { expect, test } from "./scenarios/test";
import { catalog } from "./catalog";
import { labCases } from "./cases";
import { readSelection } from "./selection";
import { foundationMessages } from "./foundationMessages";

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
          await expect(page.locator("html")).toHaveAttribute("data-lab-ready", "true", {
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
          if (entry.storyId === "form") {
            const name = page.getByRole("textbox", { name: copy.name, exact: true });
            const error = page.getByRole("alert");
            const review = page.getByRole("group", { name: copy.review, exact: true });
            const save = page.getByRole("button", { name: copy.save, exact: true });
            if (entry.variantId === "editable") {
              await expect(name).not.toHaveAttribute("aria-invalid", "true");
              await expect(error).toBeHidden();
              await save.click();
            }
            await expect(name).toHaveAttribute("aria-invalid", "true");
            await expect(name).toHaveAccessibleDescription(copy.invalid);
            await expect(error).toBeVisible();
            await expect(error).toHaveText(copy.invalid);
            await testInfo.attach(entry.id + "-invalid-" + entry.width + "px", {
              body: await page.screenshot({ fullPage: true, animations: "disabled" }),
              contentType: "image/png",
            });
            const bounds = async (locator: typeof name) => (await locator.boundingBox())!;
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
            await expect(page.getByRole("status")).toHaveText(copy.saved);
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
