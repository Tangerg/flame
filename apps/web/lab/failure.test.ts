import { describe, expect, it } from "vitest";
import { failurePresentation } from "./failure";

describe("Lab failure presentation", () => {
  it("projects scoped appearance without advancing the document or stored preferences", () => {
    const before = {
      style: document.documentElement.getAttribute("style"),
      classes: document.documentElement.className,
      locale: document.documentElement.lang,
      storage: Object.entries(window.localStorage).sort(),
    };
    const query = new URLSearchParams("locale=zh-CN&theme=dark&font-size=18&motion=off");
    const original = query.toString();
    const presentation = failurePresentation(query);
    expect(presentation.locale).toBe("zh-CN");
    expect(presentation.scheme).toBe("dark");
    expect(presentation.style.colorScheme).toBe("dark");
    expect(presentation.variables["--fs-ui-md"]).toBe("18px");
    expect(presentation.variables["--color-bg"]).toBeTruthy();
    expect(query.toString()).toBe(original);
    expect({
      style: document.documentElement.getAttribute("style"),
      classes: document.documentElement.className,
      locale: document.documentElement.lang,
      storage: Object.entries(window.localStorage).sort(),
    }).toEqual(before);
  });

  it("uses canonical fixture colors and recovers to their effective scheme", () => {
    const presentation = failurePresentation(
      new URLSearchParams({
        theme: "light",
        "custom-bg": "#102030",
        "custom-fg": "#eeeeee",
        accent: "#6f42c1",
        "font-size": "18",
      }),
      "fixture",
    );
    expect(presentation.locale).toBe("en");
    expect(presentation.scheme).toBe("dark");
    const url = new URL(presentation.href, "http://lab.test");
    expect(url.searchParams.get("theme")).toBe("dark");
    expect(url.searchParams.get("font-size")).toBe("18");
    expect(url.searchParams.has("custom-bg")).toBe(false);
    expect(presentation.variables["--color-accent"]).toBeTruthy();
  });
});
