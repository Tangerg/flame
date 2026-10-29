import { beforeEach, describe, expect, it, vi } from "vitest";
import { type HighlighterCore } from "shiki/core";
import { langFromPath, resolveLang } from "@/lib/highlight/shiki";
const { createHighlighterCore } = vi.hoisted(() => ({ createHighlighterCore: vi.fn() }));

vi.mock("shiki/core", () => ({ createHighlighterCore }));

const INHERITED_KEYS = ["constructor", "toString", "valueOf", "hasOwnProperty", "__proto__"];

describe("langFromPath", () => {
  it("maps common extensions to their Shiki language", () => {
    expect(langFromPath("a/b/main.go")).toBe("go");
    expect(langFromPath("script.py")).toBe("python");
    expect(langFromPath("src/App.tsx")).toBe("tsx");
    expect(langFromPath("lib/util.ts")).toBe("typescript");
    expect(langFromPath("main.rs")).toBe("rust");
    expect(langFromPath("style.scss")).toBe("scss");
  });

  it("recognizes well-known bare filenames", () => {
    expect(langFromPath("deploy/Dockerfile")).toBe("dockerfile");
    expect(langFromPath("Makefile")).toBe("bash");
  });

  it("falls back to text for unknown / extensionless paths", () => {
    expect(langFromPath("notes")).toBe("text");
    expect(langFromPath("data.kdl")).toBe("text");
  });

  it("is case-insensitive on the extension", () => {
    expect(langFromPath("README.MD")).toBe("markdown");
  });

  it("answers with a language tag for filenames that name an inherited member", () => {
    for (const key of INHERITED_KEYS) {
      expect(typeof langFromPath(key)).toBe("string");
      expect(typeof langFromPath(`src/${key}`)).toBe("string");
      expect(typeof langFromPath(`file.${key}`)).toBe("string");
    }
  });
});

describe("resolveLang", () => {
  const highlighter = {
    getLoadedLanguages: () => ["typescript", "bash", "cpp"],
  } as unknown as HighlighterCore;

  it("keeps a loaded tag and aliases a known short one", () => {
    expect(resolveLang(highlighter, "typescript")).toBe("typescript");
    expect(resolveLang(highlighter, "sh")).toBe("bash");
    expect(resolveLang(highlighter, "c++")).toBe("cpp");
  });

  it("degrades an un-bundled tag to text", () => {
    expect(resolveLang(highlighter, "kdl")).toBe("text");
  });

  it("degrades a tag that names an inherited member rather than throwing", () => {
    for (const key of INHERITED_KEYS) {
      expect(resolveLang(highlighter, key)).toBe("text");
    }
  });
});

describe("getHighlighter", () => {
  beforeEach(() => {
    vi.resetModules();
    createHighlighterCore.mockReset();
  });

  it("builds the highlighter once and shares it", async () => {
    const highlighter = {} as HighlighterCore;
    createHighlighterCore.mockResolvedValue(highlighter);
    const { getHighlighter } = await import("@/lib/highlight/shiki");

    await expect(getHighlighter()).resolves.toBe(highlighter);
    await expect(getHighlighter()).resolves.toBe(highlighter);
    expect(createHighlighterCore).toHaveBeenCalledTimes(1);
  });

  it("does not keep a failure, so the next code block tries again", async () => {
    const highlighter = {} as HighlighterCore;
    createHighlighterCore
      .mockRejectedValueOnce(new Error("chunk unavailable"))
      .mockResolvedValue(highlighter);
    const { getHighlighter } = await import("@/lib/highlight/shiki");

    await expect(getHighlighter()).rejects.toThrow("chunk unavailable");
    await expect(getHighlighter()).resolves.toBe(highlighter);
    expect(createHighlighterCore).toHaveBeenCalledTimes(2);
  });
});

it("disposes an initializing highlighter and creates a new instance on the next lifetime", async () => {
  vi.resetModules();
  let resolve!: (value: HighlighterCore) => void;
  const first = { dispose: vi.fn() } as unknown as HighlighterCore;
  const second = { dispose: vi.fn() } as unknown as HighlighterCore;
  createHighlighterCore
    .mockReset()
    .mockImplementationOnce(
      () =>
        new Promise<HighlighterCore>((done) => {
          resolve = done;
        }),
    )
    .mockResolvedValueOnce(second);
  const { getHighlighter, disposeHighlighter } = await import("@/lib/highlight/shiki");
  const loading = getHighlighter();
  await vi.waitFor(() => expect(createHighlighterCore).toHaveBeenCalledTimes(1));
  const disposing = disposeHighlighter();
  resolve(first);
  await loading;
  await disposing;
  expect(first.dispose).toHaveBeenCalledTimes(1);
  expect(await getHighlighter()).toBe(second);
  await disposeHighlighter();
  expect(second.dispose).toHaveBeenCalledTimes(1);
});
