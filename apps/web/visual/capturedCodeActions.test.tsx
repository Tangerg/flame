import { act, cleanup, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi, onTestFinished } from "vitest";
import { createTranslator, type ComponentContext } from "@flame/component-sdk";
import { CodeBlock } from "@flame/ui";
import { createTestClient, type TestClient } from "@/test/renderClient";
import { Markdown } from "../../../plugins/file-renderers/component/component";
import { Diagnostics } from "../../../plugins/diagnostics/component/component";
import { TrajectoryReport } from "../../../plugins/trajectory/component/report";
import { RUNTIME_AGENT_SESSION_SNAPSHOTS } from "./agentSessionSnapshots";
import { visualTrajectory } from "./trajectorySnapshot";
import { en } from "../../../plugins/trajectory/component/locales/en";
import { zh } from "../../../plugins/trajectory/component/locales/zh";
import { zhTW } from "../../../plugins/trajectory/component/locales/zh-TW";
import { ja } from "../../../plugins/trajectory/component/locales/ja";
import { ko } from "../../../plugins/trajectory/component/locales/ko";
import { es } from "../../../plugins/trajectory/component/locales/es";
import { fr } from "../../../plugins/trajectory/component/locales/fr";
import { de } from "../../../plugins/trajectory/component/locales/de";

let client: TestClient;
beforeEach(() => {
  client = createTestClient();
  onTestFinished(() => client.dispose());
  const reset = client.render(<CodeBlock label="Reset wrapping">reset</CodeBlock>);
  const unwrap = screen.queryByRole("button", { name: "Disable word wrap" });
  if (unwrap) fireEvent.click(unwrap);
  reset.unmount();
});
afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

const labels = [
  ["en", "Copy code", "Copied", "Enable word wrap", "Disable word wrap"],
  ["zh", "复制代码", "已复制", "启用自动换行", "关闭自动换行"],
  ["zh-TW", "複製程式碼", "已複製", "啟用自動換行", "關閉自動換行"],
  ["ja", "コードをコピー", "コピーしました", "折り返しを有効にする", "折り返しを無効にする"],
  ["ko", "코드 복사", "복사됨", "자동 줄 바꿈 사용", "자동 줄 바꿈 사용 안 함"],
  ["es", "Copiar código", "Copiado", "Activar ajuste de línea", "Desactivar ajuste de línea"],
  [
    "fr",
    "Copier le code",
    "Copié",
    "Activer le retour à la ligne",
    "Désactiver le retour à la ligne",
  ],
  ["de", "Code kopieren", "Kopiert", "Zeilenumbruch aktivieren", "Zeilenumbruch deaktivieren"],
] as const;

function context(locale: string): ComponentContext {
  const content = '```text\nconst marker = "reader.locale";\n```';
  return {
    component: { installationId: "captured-reader", digest: "a".repeat(64), componentId: "reader" },
    locale,
    signal: new AbortController().signal,
    file: { path: "review.md", content, startLine: 1, totalLines: 3, truncated: false },
    telemetry: {
      read: () => ({
        spans: [
          {
            id: "span",
            traceId: "trace",
            name: "reader.locale",
            kind: "internal",
            startMs: 1785484800000,
            durationMillis: 21.5,
            status: "ok",
            attrs: {},
          },
        ],
        metrics: [],
        logs: [],
      }),
      clear() {
        throw Error("Reading must not clear telemetry");
      },
    },
    get runtime(): never {
      throw Error("Captured readers must not request live data");
    },
    track() {
      throw Error("Captured readers must not start Runtime I/O");
    },
    saveFile() {
      throw Error("Captured readers must not save files");
    },
    compose() {
      throw Error("Captured readers must not compose another component");
    },
  };
}

function renderReader(kind: "file" | "trajectory" | "diagnostics", locale: string) {
  const input = context(locale);
  const row = visualTrajectory(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle).find(
    (entry) => entry.type === "model",
  )!;
  const reader =
    kind === "file" ? (
      <Markdown context={input} />
    ) : kind === "diagnostics" ? (
      <Diagnostics context={input} />
    ) : (
      <TrajectoryReport
        page={{ data: [row] }}
        pageNumber={1}
        kind="all"
        run="all"
        search=""
        onClearFilters={() => {}}
        t={createTranslator({ en, zh, "zh-TW": zhTW, ja, ko, es, fr, de }, locale)}
        locale={locale}
      />
    );
  const view = client.render(
    <>
      <input aria-label="Later draft" defaultValue="Keep this draft" />
      {reader}
    </>,
  );
  if (kind === "trajectory") {
    const record = view.container.querySelector<HTMLElement>("[data-trajectory-record]")!;
    fireEvent.click(within(record).getByRole("button", { expanded: false }));
  }
  if (kind === "diagnostics") {
    const record = screen.getByRole("button", { name: /^reader\.locale/, expanded: false });
    fireEvent.click(record);
    const body = document.getElementById(record.getAttribute("aria-controls")!)!;
    fireEvent.click(within(body).getByRole("button", { expanded: false }));
  }
  return view;
}

for (const kind of ["file", "trajectory", "diagnostics"] as const) {
  it.each(labels)(
    `keeps ${kind} code actions and feedback in %s without changing the captured source or later focus`,
    async (locale, copy, copied, wrap, unwrap) => {
      const pending = Promise.withResolvers<void>();
      const writeText = vi.fn(() => pending.promise);
      vi.stubGlobal("navigator", { clipboard: { writeText } });
      const view = renderReader(kind, locale);
      const region = view.container.querySelector<HTMLPreElement>('pre[role="region"]')!;
      expect(region).not.toBeNull();
      const source = region.textContent!;
      expect(source.length).toBeGreaterThan(0);
      fireEvent.click(screen.getByRole("button", { name: wrap }));
      expect(screen.getByRole("button", { name: unwrap })).toBeTruthy();
      expect(region.getAttribute("data-wrap")).toBe("true");
      fireEvent.click(screen.getByRole("button", { name: copy }));
      expect(writeText).toHaveBeenCalledWith(source);
      expect(screen.queryByRole("button", { name: copied })).toBeNull();
      const draft = screen.getByRole("textbox", { name: "Later draft" });
      act(() => draft.focus());

      await act(async () => pending.resolve());

      expect(screen.getByRole("button", { name: copied })).toBeTruthy();
      expect(view.container.querySelector('pre[role="region"]')).toBe(region);
      expect(region.textContent).toBe(source);
      expect(document.activeElement).toBe(draft);
      expect((draft as HTMLInputElement).value).toBe("Keep this draft");
    },
  );
}
