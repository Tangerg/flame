import { afterEach, expect, it, vi } from "vitest";
import i18next from "i18next";
import { activeLocale, mountLocaleBundle, setLocale, t } from "@/lib/i18n/index";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
import { LOCALE } from "@/features/i18n/public/extensionPoints";
import { loadPluginsForTest, resetKernelForTest } from "@/test/kernel";
import {
  contributeLocale,
  createLocaleReconciler,
  type LocaleSource,
} from "@/features/i18n/public/locales";
import { definePlugin } from "@/extensions/definePlugin";
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
  await loadPluginsForTest(
    definePlugin({
      name: "test.locale",
      setup(scope) {
        contributeLocale(scope, { id: locale, label: "Owner", load });
      },
    }),
  );
  expect(load).toHaveBeenCalledOnce();
  const stopped = resetKernelForTest();
  dictionary.resolve({ [key]: "retired" });
  await stopped;
  expect(t(key)).toBe(key);
});

it("shares one lazy activation and withdraws its dictionary on disposal", async () => {
  const load = vi.fn(async () => ({ [key]: "owned" }));
  setLocale(locale);
  await loadPluginsForTest(
    definePlugin({
      name: "test.locale",
      setup(scope) {
        contributeLocale(scope, { id: locale, label: "Owner", load });
      },
    }),
  );
  const spec = lookupExtensionByKey(LOCALE, locale)!;
  await Promise.all([spec.activate(), spec.activate()]);
  expect(load).toHaveBeenCalledOnce();
  expect(t(key)).toBe("owned");
  await resetKernelForTest();
  expect(t(key)).toBe(key);
});

it("shares each load attempt and permits a new attempt after failure", async () => {
  const failure = new Error("dictionary unavailable");
  const load = vi
    .fn()
    .mockRejectedValueOnce(failure)
    .mockResolvedValue({ [key]: "recovered" });
  await loadPluginsForTest(
    definePlugin({
      name: "test.locale",
      setup(scope) {
        contributeLocale(scope, { id: locale, label: "Owner", load });
      },
    }),
  );
  const spec = lookupExtensionByKey(LOCALE, locale)!;
  const first = spec.activate();
  expect(spec.activate()).toBe(first);
  await expect(first).rejects.toBe(failure);

  const retry = spec.activate();
  expect(spec.activate()).toBe(retry);
  await retry;
  expect(load).toHaveBeenCalledTimes(2);
  expect(spec.activate()).toBe(retry);
  setLocale(locale);
  expect(t(key)).toBe("recovered");
});

it("does not publish a retried dictionary after its source retires", async () => {
  const dictionary = Promise.withResolvers<Record<string, string>>();
  const load = vi
    .fn()
    .mockRejectedValueOnce(new Error("first read failed"))
    .mockImplementationOnce(() => dictionary.promise);
  await loadPluginsForTest(
    definePlugin({
      name: "test.locale",
      setup(scope) {
        contributeLocale(scope, { id: locale, label: "Owner", load });
      },
    }),
  );
  const spec = lookupExtensionByKey(LOCALE, locale)!;
  await expect(spec.activate()).rejects.toThrow("first read failed");
  const retry = spec.activate();
  const settled = Promise.allSettled([retry]);
  const stopped = resetKernelForTest();
  dictionary.resolve({ [key]: "retired retry" });
  await stopped;
  await settled;
  expect(load).toHaveBeenCalledTimes(2);
  setLocale(locale);
  expect(t(key)).toBe(key);
});

async function localeCatalog() {
  let reconcile!: (sources: readonly LocaleSource[]) => Promise<void>;
  const report = vi.fn();
  await loadPluginsForTest(
    definePlugin({
      name: "test.locale-catalog",
      setup(scope) {
        reconcile = createLocaleReconciler(scope.lifetime("catalog"), report);
      },
    }),
  );
  return { reconcile, report };
}

it("withdraws all conflicting sources and resumes the surviving source without changing preference", async () => {
  const { reconcile, report } = await localeCatalog();
  const original = {
    key: "installation-a:release-a",
    tag: locale,
    title: "Original",
    load: vi.fn(async () => ({ [key]: "original" })),
  };
  const replacement = {
    key: "installation-b:release-b",
    tag: locale,
    title: "Replacement",
    load: vi.fn(async () => ({ [key]: "replacement" })),
  };
  setLocale(locale);
  await reconcile([original]);
  await lookupExtensionByKey(LOCALE, locale)!.activate();
  expect(t(key)).toBe("original");
  await reconcile([original, replacement]);
  expect(lookupExtensionByKey(LOCALE, locale)).toBeUndefined();
  expect(t(key)).toBe(key);
  expect(activeLocale()).toBe(locale);
  expect(replacement.load).not.toHaveBeenCalled();
  expect(report).toHaveBeenCalledOnce();
  expect(report.mock.calls[0]?.[0].message).toContain("conflicting sources");
  await reconcile([replacement, original]);
  expect(report).toHaveBeenCalledOnce();
  await reconcile([replacement]);
  await lookupExtensionByKey(LOCALE, locale)!.activate();
  expect(t(key)).toBe("replacement");
  expect(activeLocale()).toBe(locale);
  await reconcile([]);
  expect(t(key)).toBe(key);
  expect(activeLocale()).toBe(locale);
});

it("joins a retiring release and refuses its late dictionary before admitting its replacement", async () => {
  const { reconcile } = await localeCatalog();
  const pending = Promise.withResolvers<Record<string, string>>();
  const old = {
    key: "installation:old-release",
    tag: locale,
    title: "Original",
    load: vi.fn(() => pending.promise),
  };
  const next = {
    key: "installation:new-release",
    tag: locale,
    title: "Next",
    load: vi.fn(async () => ({ [key]: "next" })),
  };
  setLocale(locale);
  await reconcile([old]);
  expect(old.load).toHaveBeenCalledOnce();
  const replacing = reconcile([next]);
  pending.resolve({ [key]: "retired" });
  await replacing;
  await lookupExtensionByKey(LOCALE, locale)!.activate();
  expect(t(key)).toBe("next");
  expect(next.load).toHaveBeenCalledOnce();
});

it("retains one source across unchanged refreshes and never replaces English", async () => {
  const { reconcile, report } = await localeCatalog();
  const source = {
    key: "installation:release",
    tag: locale,
    title: "Original",
    load: vi.fn(async () => ({ [key]: "original" })),
  };
  await reconcile([source]);
  const first = lookupExtensionByKey(LOCALE, locale)!;
  await first.activate();
  await reconcile([source]);
  expect(lookupExtensionByKey(LOCALE, locale)).toBe(first);
  await first.activate();
  expect(source.load).toHaveBeenCalledOnce();
  const english = { ...source, key: "replacement-english", tag: "en" };
  await reconcile([source, english]);
  expect(report).toHaveBeenCalledOnce();
  expect(lookupExtensionByKey(LOCALE, "en")).toBeUndefined();
  expect(t("common.loading")).toBe("Loading");
});

it("cancels an unfinished catalog read when its source withdraws", async () => {
  const { reconcile } = await localeCatalog();
  let reading!: AbortSignal;
  const source: LocaleSource = {
    key: "installation:pending-release",
    tag: locale,
    title: "Pending",
    load(signal) {
      reading = signal;
      return new Promise((_resolve, reject) => {
        signal.addEventListener("abort", () => reject(signal.reason), { once: true });
      });
    },
  };
  setLocale(locale);
  await reconcile([source]);
  expect(reading.aborted).toBe(false);
  await reconcile([]);
  expect(reading.aborted).toBe(true);
  expect(lookupExtensionByKey(LOCALE, locale)).toBeUndefined();
  expect(t(key)).toBe(key);
  expect(activeLocale()).toBe(locale);
});
