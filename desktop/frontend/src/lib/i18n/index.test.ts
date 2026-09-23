import { afterEach, describe, expect, it } from "vitest";
import { activeLocale, setLocale } from "./index";

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
});
