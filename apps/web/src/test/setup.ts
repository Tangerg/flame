import { createElement, Fragment, type ReactNode } from "react";
import { afterEach, beforeEach, vi } from "vitest";
import "fake-indexeddb/auto";
import { IDBFactory } from "fake-indexeddb";
import { MotionGlobalConfig } from "motion/react";
import { usePluginErrorStore } from "@/extensions/errors";
import { resetKernelForTest } from "@/test/kernel";
MotionGlobalConfig.skipAnimations = true;

beforeEach(async () => {
  await resetKernelForTest();
  globalThis.indexedDB = new IDBFactory();
  localStorage.clear();
  usePluginErrorStore.setState({ log: [] });
});

afterEach(() => {
  try {
    localStorage.clear();
  } catch {}
});

// Layout and worker execution are exercised by the production browser suite.
// Happy DOM business tests render captured material at these public boundaries.
vi.mock("@/lib/highlight/PierreFile", () => ({
  PierreFile: ({
    source,
    startLine = 1,
    targetLine,
    intent,
  }: {
    source: string;
    startLine?: number;
    targetLine?: number;
    intent?: object | null;
  }) =>
    createElement(
      "pre",
      { "data-presented-target": intent && targetLine && targetLine > 0 ? targetLine : undefined },
      createElement(
        "code",
        null,
        source
          .split("\n")
          .map((line, index, lines) =>
            createElement(
              "span",
              { key: index, "data-quote-line": index + startLine, "data-reader-line": index },
              line + (index < lines.length - 1 ? "\n" : ""),
            ),
          ),
      ),
    ),
}));
vi.mock("@flame/ui", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@flame/ui")>()),
  ReadingList: function ReadingList<T>({
    data,
    keyExtractor,
    renderItem,
    header,
    footer,
    label,
    role = "region",
  }: {
    data: readonly T[];
    keyExtractor(item: T): string;
    renderItem(item: T, index: number): ReactNode;
    header?: ReactNode;
    footer?: ReactNode;
    label: string;
    role?: "region" | "group";
  }) {
    return createElement(
      "div",
      { role, "aria-label": label, tabIndex: 0 },
      header,
      data.map((item, index) =>
        createElement(Fragment, { key: keyExtractor(item) }, renderItem(item, index)),
      ),
      footer,
    );
  },
}));
