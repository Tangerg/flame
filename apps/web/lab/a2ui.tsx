import { createHighlighter, disposeHighlighter } from "@pierre/diffs";
import { WorkerPoolManager } from "@pierre/diffs/worker";
import workerSource from "@pierre/diffs/worker/worker-portable.js?raw";
import { Activity, useEffect, useMemo, useRef, useState } from "react";
import { createRoot } from "react-dom/client";
import { createFlameClient } from "@flame/runtime-client";
import { createMemoryTransport } from "@flame/runtime-client/testing";
import { type ComponentContext, type ComponentDefinition } from "@flame/component-sdk";
import { Button, ContentVisibility, ReadingList, type ReadingListHandle } from "@flame/ui";
import { Presentation } from "../../../plugins/genui/contracts/document";
import { interfaceRequest } from "../../../plugins/genui/contracts/fixture";
import { createLabEnvironment } from "./environment";
import { unavailableComponentNotifications } from "@/test/unavailableNotifications";
import { PierreFile } from "@/lib/highlight/PierreFile";
import { disposeOnHmr } from "@/lib/hmr";
import "@flame/ui/styles.css";
import "@/styles/globals.css";
import "@/styles/markdown.css";
const definition = Object.values(
  import.meta.glob<{ GenUIResult: ComponentDefinition }>(
    "../../../plugins/genui/package/io.github.tangerg.flame/components/component.js",
    { eager: true },
  ),
)[0]!.GenUIResult;
const lifetime = new AbortController();
const runtime = createFlameClient(createMemoryTransport());
const component = {
  installationId: "fixture-genui",
  digest: "a".repeat(64),
  componentId: "report",
};
const GenUI = definition.create({ runtimeIdentity: runtime, component, signal: lifetime.signal });
const request = interfaceRequest(true);
const result = Presentation.accept(request, []).json();
const environment = await createLabEnvironment({
  theme: "light",
  locale: "en",
  motion: "off",
  fontSize: 16,
});
function Document() {
  const owner = useMemo(() => new AbortController(), []);
  useEffect(() => () => owner.abort(), [owner]);
  const context = useMemo<ComponentContext>(
    () => ({
      component,
      runtime,
      locale: "en",
      signal: owner.signal,
      telemetry: { read: () => ({ spans: [], metrics: [], logs: [] }), clear() {} },
      toolResult: {
        runId: "original-run",
        id: "original-item",
        name: "ui_present",
        fn: "ui_present",
        args: "{}",
        status: "ok",
        result,
      },
      interaction: {
        source: { runId: "original-run", itemId: "original-item" },
        submit: async (input) => {
          document.querySelector("[data-submission]")!.textContent = JSON.stringify(input);
          return {
            status: "accepted",
            sessionId: "original-session",
            runId: "followup-run",
            userItemId: "followup-item",
          };
        },
      },
      track: (operation) => operation(owner.signal),
      saveFile: async () => "requested",
      notifications: unavailableComponentNotifications,
      subscribeRootRunSettlements() {
        throw new Error("unexpected settlement observation");
      },
      registerCommand() {
        throw new Error("unexpected command registration");
      },
      compose: () => null,
    }),
    [owner],
  );
  return <GenUI context={context} />;
}
async function highlightsProbe(worker: boolean): Promise<string> {
  try {
    if (!worker) {
      try {
        const highlighter = await createHighlighter("highlights");
        highlighter.dispose();
      } finally {
        await disposeHighlighter();
      }
    } else {
      const pool = new WorkerPoolManager(
        {
          poolSize: 1,
          workerFactory: () => {
            const url = URL.createObjectURL(new Blob([workerSource], { type: "text/javascript" }));
            try {
              return new Worker(url, { type: "module" });
            } finally {
              URL.revokeObjectURL(url);
            }
          },
        },
        { preferredHighlighter: "highlights", theme: "pierre-dark" },
      );
      try {
        await pool.primeFileHighlightCache({
          name: "probe.ts",
          contents: "const exact = 42;",
          lang: "typescript",
          cacheKey: "csp-highlights-probe",
        });
      } finally {
        pool.terminate();
      }
    }
    return "unexpected success";
  } catch (error) {
    return "blocked: " + String(error);
  }
}
function Fixture() {
  const ref = useRef<ReadingListHandle>(null);
  const [growth, setGrowth] = useState(0);
  const [visible, setVisible] = useState(true);
  const [prepended, setPrepended] = useState(false);
  const [code, setCode] = useState(false);
  const [probes, setProbes] = useState<readonly string[]>([]);
  const rows = [
    ...(prepended ? ["prepended"] : []),
    ...Array.from({ length: 120 }, (_, index) => String(index)),
  ];
  return (
    <main style={{ height: "100dvh", display: "flex", flexDirection: "column" }}>
      <nav aria-label="Acceptance controls">
        <Button
          onClick={() => {
            void Promise.all([highlightsProbe(false), highlightsProbe(true)]).then(setProbes);
          }}
        >
          Probe Highlights
        </Button>
        <Button onClick={() => setCode(true)}>Render code</Button>
        <Button
          onClick={() =>
            void ref.current?.scrollToKey({ key: "70", viewPosition: 0.5, animated: false })
          }
        >
          Show interface
        </Button>
        <Button onClick={() => void ref.current?.scrollToKey({ key: "0", animated: false })}>
          Show start
        </Button>
        <Button onClick={() => setGrowth((value) => value + 1)}>Reflow upper row</Button>
        <Button onClick={() => setPrepended(true)}>Prepend</Button>
        <Button onClick={() => setVisible((value) => !value)}>Toggle activity</Button>
      </nav>
      <output data-highlights aria-label="Highlights CSP probes">
        {probes.join("\n")}
      </output>
      <output data-submission aria-label="Captured submission" />
      {code && <PierreFile source={'const exact = "9007199254740993";'} language="typescript" />}
      <ContentVisibility visible={visible}>
        <Activity mode={visible ? "visible" : "hidden"}>
          <ReadingList
            ref={ref}
            label="Acceptance conversation"
            data={rows}
            keyExtractor={(row) => row}
            renderItem={(row) =>
              row === "70" ? (
                <Document />
              ) : (
                <article style={{ padding: 16, minHeight: row === "69" ? 96 + growth * 1200 : 96 }}>
                  <p>{`Captured row ${row}`}</p>
                </article>
              )
            }
          />
        </Activity>
      </ContentVisibility>
    </main>
  );
}
const root = createRoot(document.getElementById("root")!);
root.render(
  <environment.Wrapper>
    <Fixture />
  </environment.Wrapper>,
);
const dispose = () => {
  root.unmount();
  lifetime.abort();
  void runtime.close();
  void environment.dispose();
};
window.addEventListener("pagehide", dispose, { once: true });
disposeOnHmr(dispose);
