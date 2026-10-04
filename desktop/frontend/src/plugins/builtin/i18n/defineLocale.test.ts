import { afterEach, expect, it, vi } from "vitest";
import i18next from "i18next";
import { mountLocaleBundle, setLocale, t } from "@/lib/i18n";
import { lookupExtensionByKey } from "@/plugins/sdk/selectors/extensions";
import { LOCALE } from "@/plugins/sdk/kernelPoints";
import { loadPluginsForTest, resetKernelForTest } from "@/plugins/sdk/testKernel";
import { defineLocale } from "./defineLocale";

const locale = "ownertest";
const key = "locale.owner-proof";

afterEach(async () => {
  await resetKernelForTest();
  i18next.removeResourceBundle(locale, "translation");
  setLocale("en");
});

it("refuses to replace another source's dictionary", () => {
  const unmount = mountLocaleBundle(locale, { [key]: "original" });
  expect(() => mountLocaleBundle(locale, { [key]: "replacement" })).toThrow(
    `locale "${locale}" already has a dictionary`,
  );
  setLocale(locale);
  expect(t(key)).toBe("original");
  unmount();
});

it("does not publish a dictionary after its source retires", async () => {
  const dictionary = Promise.withResolvers<Record<string, string>>();
  const load = vi.fn(() => dictionary.promise);
  setLocale(locale);
  await loadPluginsForTest(defineLocale({ id: locale, label: "Owner", load }));
  expect(load).toHaveBeenCalledOnce();
  const stopped = resetKernelForTest();
  dictionary.resolve({ [key]: "retired" });
  await stopped;
  expect(t(key)).toBe(key);
});

it("shares one lazy activation and withdraws its dictionary on disposal", async () => {
  const load = vi.fn(async () => ({ [key]: "owned" }));
  setLocale(locale);
  await loadPluginsForTest(defineLocale({ id: locale, label: "Owner", load }));
  const spec = lookupExtensionByKey(LOCALE, locale)!;
  await Promise.all([spec.activate(), spec.activate()]);
  expect(load).toHaveBeenCalledOnce();
  expect(t(key)).toBe("owned");
  await resetKernelForTest();
  expect(t(key)).toBe(key);
});
