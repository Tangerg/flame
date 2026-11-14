import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { screen, within } from "@testing-library/react";
import { showBrowserStartupFailure } from "./browserSupportFailure";
import { startupCopy } from "./startupCopy.generated";
import { LOCALE_STORAGE_KEY } from "@/lib/i18n/locale";

let root: HTMLElement;
let previousLanguage: string;
beforeEach(() => {
  root = document.createElement("div");
  document.body.append(root);
  previousLanguage = document.documentElement.lang;
  vi.spyOn(navigator, "language", "get").mockReturnValue("en-US");
});
afterEach(() => {
  root.remove();
  document.documentElement.lang = previousLanguage;
  localStorage.removeItem(LOCALE_STORAGE_KEY);
  vi.restoreAllMocks();
});

it.each(Object.keys(startupCopy) as Array<keyof typeof startupCopy>)(
  "renders the Host's %s startup diagnosis before runtime locale activation",
  (locale) => {
    localStorage.setItem(LOCALE_STORAGE_KEY, locale);
    showBrowserStartupFailure(root, { status: "unsupported", missing: ["Promise.withResolvers"] });
    const copy = startupCopy[locale];
    const alert = screen.getByRole("alert", { name: copy["startup.failed.title"] });
    expect(within(alert).getByRole("heading", { name: copy["startup.failed.title"] })).toBeTruthy();
    expect(alert.textContent).toContain(copy["startup.unsupported.body"]);
    expect(alert.textContent).toContain(copy["startup.unsupported.browsers"]);
    expect(alert.textContent).toContain(copy["startup.unsupported.features"]);
    expect(within(alert).getByText("Promise.withResolvers").textContent).toBe(
      "Promise.withResolvers",
    );
    expect(within(alert).queryByRole("button")).toBeNull();
    expect(alert.tabIndex).toBe(0);
    expect(document.documentElement.lang).toBe(locale === "zh" ? "zh-CN" : locale);
  },
);

it.each(["private-locale", "__proto__"])(
  "keeps an unavailable preference %s while using English diagnostics",
  (locale) => {
    localStorage.setItem(LOCALE_STORAGE_KEY, locale);
    showBrowserStartupFailure(root, { status: "unsupported", missing: ["AbortSignal.any"] });
    expect(screen.getByRole("alert").textContent).toContain(
      startupCopy.en["startup.unsupported.body"],
    );
    expect(document.documentElement.lang).toBe("en");
    expect(localStorage.getItem(LOCALE_STORAGE_KEY)).toBe(locale);
  },
);

it("offers a localized module failure with selectable literal details without taking focus", () => {
  localStorage.setItem(LOCALE_STORAGE_KEY, "zh");
  const elsewhere = document.createElement("button");
  elsewhere.textContent = "Elsewhere";
  document.body.append(elsewhere);
  elsewhere.focus();
  const cause = new Error("<script>invalid module</script>\noriginal import failure");
  try {
    showBrowserStartupFailure(root, { status: "failed", cause });
    const alert = screen.getByRole("alert");
    expect(alert.textContent).toContain(startupCopy.zh["startup.loadFailed.body"]);
    const reader = within(alert).getByRole("region", {
      name: startupCopy.zh["plugins.renderFailed.details"],
    });
    expect(reader.textContent).toBe(cause.message);
    expect(reader.tabIndex).toBe(0);
    expect(
      within(alert).getByRole("button", { name: startupCopy.zh["common.retry"] }),
    ).toBeTruthy();
    expect(root.querySelector("script")).toBeNull();
    expect(document.activeElement).toBe(elsewhere);
  } finally {
    elsewhere.remove();
  }
});
