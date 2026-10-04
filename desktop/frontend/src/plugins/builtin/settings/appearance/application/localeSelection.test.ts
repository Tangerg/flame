import { afterEach, expect, it } from "vitest";
import { activeLocale, setLocale } from "@/lib/i18n";
import { selectLocale } from "./localeSelection";

afterEach(() => setLocale("en"));

it("advances the requested preference before its lazy dictionary settles", async () => {
  const activation = Promise.withResolvers<void>();
  const selecting = selectLocale({
    id: "ja",
    label: "Japanese",
    activate: () => activation.promise,
  });
  const requested = activeLocale();
  activation.resolve();
  await selecting;
  expect(requested).toBe("ja");
});

it("does not let a late dictionary replace a newer selection", async () => {
  const first = Promise.withResolvers<void>();
  const selectingFirst = selectLocale({
    id: "ja",
    label: "Japanese",
    activate: () => first.promise,
  });
  await selectLocale({ id: "de", label: "German", activate: async () => undefined });
  first.resolve();
  await selectingFirst;
  expect(activeLocale()).toBe("de");
  expect(document.documentElement.lang).toBe("de");
  expect(localStorage.getItem("flame.locale")).toBe("de");
});

it("reports failed activation while preserving the requested preference", async () => {
  const failure = new Error("dictionary could not be loaded");
  await expect(
    selectLocale({
      id: "ja",
      label: "Japanese",
      activate: async () => {
        throw failure;
      },
    }),
  ).rejects.toBe(failure);
  expect(activeLocale()).toBe("ja");
});
