import { beforeEach as prepareTestClient, onTestFinished as finishTestClient } from "vitest";
import { createTestClient, type TestClient } from "@/test/renderClient";
let testClient: TestClient;
prepareTestClient(() => {
  const owned = createTestClient();
  testClient = owned;
  finishTestClient(() => owned.dispose());
});
import { act, cleanup, fireEvent, screen, waitFor } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { activeLocale, setLocale, useT } from "@/lib/i18n/index";
import { en } from "@/lib/i18n/locales/en";
import { defineLocale } from "@/features/i18n/defineLocale";
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

const recoveryLocale = "fr-CA";
const recoveredDictionary = {
  ...en,
  "settings.language.label": "Recovered language",
  "settings.appearance.title": "Recovered reading context",
};

function ReadingContext() {
  const t = useT();
  return <p>{t("settings.appearance.title")}</p>;
}

async function failDictionary(load: () => Promise<Record<string, string>>) {
  await loadPluginsForTest(
    defineLocale({ id: "en", label: "English" }),
    defineLocale({ id: recoveryLocale, label: "Recovery language", load }),
  );
  testClient.render(
    <>
      <LanguageSection />
      <ReadingContext />
      <textarea aria-label="Independent draft" />
    </>,
  );
  fireEvent.click(screen.getByRole("button", { name: "Language" }));
  fireEvent.click(await screen.findByRole("menuitemradio", { name: "Recovery language" }));
  expect(await screen.findByText("dictionary unavailable")).toBeTruthy();
  return screen.getByRole("button", { name: "Retry" });
}

it("retries the selected language, reports pending and updates independent reading content", async () => {
  const dictionary = Promise.withResolvers<Record<string, string>>();
  const load = vi
    .fn()
    .mockRejectedValueOnce(new Error("dictionary unavailable"))
    .mockImplementationOnce(() => dictionary.promise);
  const retry = await failDictionary(load);
  retry.focus();
  fireEvent.click(retry);
  expect(screen.getByRole("status").textContent).toContain("Loading");
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Language" }));
  await act(async () => dictionary.resolve(recoveredDictionary));
  expect(await screen.findByText("Recovered reading context")).toBeTruthy();
  expect(document.activeElement).toBe(screen.getByRole("button", { name: "Recovered language" }));
  expect(activeLocale()).toBe(recoveryLocale);
  expect(localStorage.getItem("flame.locale")).toBe(recoveryLocale);
  expect(load).toHaveBeenCalledTimes(2);
  expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
});

it("keeps a later independent draft and focus when the retried dictionary arrives", async () => {
  const dictionary = Promise.withResolvers<Record<string, string>>();
  const load = vi
    .fn()
    .mockRejectedValueOnce(new Error("dictionary unavailable"))
    .mockImplementationOnce(() => dictionary.promise);
  fireEvent.click(await failDictionary(load));
  const draft = screen.getByRole("textbox", { name: "Independent draft" }) as HTMLTextAreaElement;
  fireEvent.change(draft, { target: { value: "Keep this later draft." } });
  draft.focus();
  draft.setSelectionRange(5, 9);
  await act(async () => dictionary.resolve(recoveredDictionary));
  expect(await screen.findByText("Recovered reading context")).toBeTruthy();
  expect(document.activeElement).toBe(draft);
  expect(draft.value).toBe("Keep this later draft.");
  expect([draft.selectionStart, draft.selectionEnd]).toEqual([5, 9]);
});

it("does not let an older retry replace a later language choice", async () => {
  const dictionary = Promise.withResolvers<Record<string, string>>();
  const load = vi
    .fn()
    .mockRejectedValueOnce(new Error("dictionary unavailable"))
    .mockImplementationOnce(() => dictionary.promise);
  fireEvent.click(await failDictionary(load));
  fireEvent.click(screen.getByRole("button", { name: "Language" }));
  fireEvent.click(await screen.findByRole("menuitemradio", { name: "English" }));
  await waitFor(() => expect(activeLocale()).toBe("en"));
  await act(async () => dictionary.resolve(recoveredDictionary));
  expect(activeLocale()).toBe("en");
  expect(localStorage.getItem("flame.locale")).toBe("en");
  expect(screen.getByRole("button", { name: "Language" }).textContent).toContain("English");
  expect(screen.queryByText("Recovered reading context")).toBeNull();
  expect(screen.queryByRole("button", { name: "Retry" })).toBeNull();
});
