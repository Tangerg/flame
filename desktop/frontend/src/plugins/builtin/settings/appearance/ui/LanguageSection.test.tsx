import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
import { activeLocale, setLocale } from "@/lib/i18n";
import { definePlugin } from "@/plugins/sdk";
import { LOCALE } from "@/plugins/sdk/kernelPoints";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { LanguageSection } from "./LanguageSection";

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
  render(<LanguageSection />);
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
  render(<LanguageSection />);
  fireEvent.click(screen.getByRole("button", { name: "Language" }));
  fireEvent.click(await screen.findByRole("menuitem", { name: "Japanese" }));
  expect(await screen.findByText("dictionary unavailable")).toBeTruthy();
  expect(activeLocale()).toBe("ja");
});
