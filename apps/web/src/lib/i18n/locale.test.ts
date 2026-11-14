import { afterEach, expect, it, vi } from "vitest";
import { browserLocale, initialLocale, localeToBcp47, LOCALE_STORAGE_KEY } from "./locale";
import { mockLocalStorage } from "@/test/localStorage";

afterEach(() => {
  vi.restoreAllMocks();
  localStorage.removeItem(LOCALE_STORAGE_KEY);
});

it("reads the saved preference without replacing a custom locale", () => {
  localStorage.setItem(LOCALE_STORAGE_KEY, "private-language");
  vi.spyOn(navigator, "language", "get").mockReturnValue("ja-JP");
  expect(initialLocale()).toBe("private-language");
  expect(localStorage.getItem(LOCALE_STORAGE_KEY)).toBe("private-language");
});

it("uses the browser language when no preference was saved", () => {
  vi.spyOn(navigator, "language", "get").mockReturnValue("zh-HK");
  expect(initialLocale()).toBe("zh-TW");
  expect(localStorage.getItem(LOCALE_STORAGE_KEY)).toBeNull();
});

it("keeps locale detection available when preference storage refuses access", () => {
  mockLocalStorage().storage.getItem.mockImplementation(() => {
    throw new DOMException("storage unavailable", "SecurityError");
  });
  vi.spyOn(navigator, "language", "get").mockReturnValue("fr-CA");
  expect(initialLocale()).toBe("fr");
});

it("keeps Chinese script selection distinct from the document language tag", () => {
  expect(browserLocale("zh-Hans-HK")).toBe("zh");
  expect(localeToBcp47("zh")).toBe("zh-CN");
  expect(localeToBcp47(browserLocale("zh-Hant"))).toBe("zh-TW");
});
