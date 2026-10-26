import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { cleanup, fireEvent, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { activeLocale, setLocale } from "@/lib/i18n/index";
import { definePlugin } from "@/extensions/definePlugin";
import { LOCALE } from "@/features/i18n/public/extensionPoints";
import { loadPluginsForTest } from "@/test/kernel";
import { LanguageSection } from "@/features/i18n/public/LanguageSection";
afterEach(() => {
  cleanup();
  setLocale("en");
});

it("does not label an unavailable preference as another registered locale", async () => {
  await loadPluginsForTest(
    definePlugin({
      name: "test.locale-catalog",
      setup(ctx) {
        ctx.contribute(LOCALE, { id: "en", label: "English", activate: async () => undefined });
      },
    }),
  );
  setLocale("und");
  testClient.render(<LanguageSection />);
  expect(screen.getByRole("button", { name: "Language" }).textContent).toContain("und");
});

it("shows a failed dictionary activation without discarding the selected preference", async () => {
  await loadPluginsForTest(
    definePlugin({
      name: "test.locale-selection",
      setup(ctx) {
        ctx.contribute(LOCALE, { id: "en", label: "English", activate: async () => undefined });
        ctx.contribute(LOCALE, {
          id: "ja",
          label: "Japanese",
          activate: async () => {
            throw new Error("dictionary unavailable");
          },
        });
      },
    }),
  );
  testClient.render(<LanguageSection />);
  fireEvent.click(screen.getByRole("button", { name: "Language" }));
  fireEvent.click(await screen.findByRole("menuitemradio", { name: "Japanese" }));
  expect(await screen.findByText("dictionary unavailable")).toBeTruthy();
  expect(activeLocale()).toBe("ja");
});
