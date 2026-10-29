import { afterEach, describe, expect, it } from "vitest";
import { createElement } from "react";
import { act, render, screen } from "@testing-library/react";
import { activeLocale, mountLocaleBundle, setLocale, useT } from "@/lib/i18n/index";
afterEach(async () => {
  setLocale("en");
  await Promise.resolve();
});

describe("the document's declared language", () => {
  it("follows the active locale", async () => {
    setLocale("ja");
    await Promise.resolve();
    expect(document.documentElement.lang).toBe("ja");

    setLocale("de");
    await Promise.resolve();
    expect(document.documentElement.lang).toBe("de");
  });

  it("gives Chinese the region its catalog actually is", async () => {
    setLocale("zh");
    await Promise.resolve();
    expect(document.documentElement.lang).toBe("zh-CN");

    setLocale("zh-TW");
    await Promise.resolve();
    expect(document.documentElement.lang).toBe("zh-TW");
  });
});

describe("locale selection identity", () => {
  it("preserves the requested locale while its lazy dictionary falls back to English", async () => {
    setLocale("fixture-locale-without-a-bundle");
    await Promise.resolve();

    expect(activeLocale()).toBe("fixture-locale-without-a-bundle");
  });

  it("refreshes mounted text when its requested dictionary arrives or retires", async () => {
    function Label() {
      const t = useT();
      return createElement("p", null, t("callout.note"));
    }
    setLocale("fr-x-late");
    const view = render(createElement(Label));
    let remove: (() => void) | undefined;
    try {
      expect(screen.getByText("Note")).toBeTruthy();
      await act(async () => {
        remove = mountLocaleBundle("fr-x-late", { "callout.note": "Deferred host label" });
      });
      expect(screen.getByText("Deferred host label")).toBeTruthy();
      expect(activeLocale()).toBe("fr-x-late");
      await act(async () => {
        remove!();
        remove = undefined;
      });
      expect(screen.getByText("Note")).toBeTruthy();
      expect(activeLocale()).toBe("fr-x-late");
    } finally {
      view.unmount();
      remove?.();
    }
  });
});
