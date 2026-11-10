import { createVisualClient } from "./client";
import { disposeOnHmr } from "@/lib/hmr";
import { createFlameClient, type FlameClient } from "@flame/runtime-client";
import {
  VISUAL_CAPABILITIES,
  VISUAL_FILE_MENTION_DRAFT,
  type VisualFileListState,
} from "./agentFixtureFacts";
import { type ServerCapabilities, type Diff } from "@flame/runtime-client/wire";
import {
  VISUAL_REMOVE_SCHEDULE_EVENT,
  createVisualRuntimeTransport,
} from "./createVisualRuntimeTransport";
import { LONG_MODEL_FACTS, SCROLLING_MODEL_FACTS } from "./runtimeSnapshots";
import { VISUAL_LONG_SKILL_DOCUMENTS } from "./skillSnapshots";
import { VISUAL_LONG_MEMORY_PAGE, VISUAL_LONG_USER_MEMORY_PAGE } from "./componentSnapshots";
import { definePlugin } from "@/extensions/definePlugin";
import { createRoot } from "react-dom/client";
import { type ReactNode } from "react";
import { FIXTURE_DEFAULT_LOCALE, fixtureColors, fixtureTheme } from "../selection";
import { QueryClientProvider } from "@tanstack/react-query";
import { MotionConfig } from "motion/react";
import "@flame/ui/styles.css";
import { CodeHighlightProvider, TooltipProvider, FontScaleProvider } from "@flame/ui";
import { HighlightedCodeSource } from "@/lib/highlight/HighlightedCodeSource";
import { queryClient } from "@/lib/queryClient";
import { LOCALE } from "@/features/i18n/public/extensionPoints";
import { lookupExtensionByKey } from "@/extensions/selectors/extensions";
import { selectLocale } from "@/features/i18n/application/localeSelection";
import { uiTypeLadderCssVariables } from "@/features/theme/kit/typeLadder";
import { UI_DENSITY_MODES } from "@/features/theme/kit/appearance";
import { SIDEBAR_DEFAULT_WIDTH_PX } from "@/lib/shellGeometry";
import { VisualFoundationFixture } from "./VisualFoundationFixture";
import {
  VISUAL_AGENT_STATES,
  RUNTIME_AGENT_SESSION_SNAPSHOTS,
  type RuntimeAgentSessionSnapshot,
  type VisualAgentState,
  delegatedApprovalSnapshot,
} from "./agentSessionSnapshots";
import {
  VISUAL_WORK_INDEX_STATES,
  isVisualShellOverlay,
  type VisualShellOverlay,
  type VisualWorkIndexState,
} from "./shellFixtureStates";
import {
  isVisualSettingsPane,
  isVisualWorkspaceState,
  type VisualSettingsPane,
  type VisualWorkspaceState,
} from "./workspaceFixtureStates";
import "@/styles/markdown.css";
import "@/styles/overlays.css";
import "@/styles/globals.css";
import { loadPluginsForTest } from "@/test/kernel";
import { VISUAL_NOW } from "./agentFixtureFacts";
import { fixtureMotionScale } from "./motion";
import { installFixtureClock } from "./fixtureClock";
const VISUAL_CLOCK_STARTED_AT = performance.now();

const restoreFixtureClock = installFixtureClock(
  () => VISUAL_NOW + Math.floor(performance.now() - VISUAL_CLOCK_STARTED_AT),
);

let currentRuntimeClient: FlameClient | undefined;
export const visualClient = createVisualClient(() => {
  if (!currentRuntimeClient) throw new Error("Visual Runtime client is not configured");
  return currentRuntimeClient;
});

const query = new URLSearchParams(window.location.search);
const requestedFileListState = query.get("file-list");
const fileListState: VisualFileListState | undefined =
  requestedFileListState === "error" || requestedFileListState === "loading"
    ? requestedFileListState
    : undefined;

const theme = fixtureTheme(query.get("theme"));

const sidebarOpen = query.get("sidebar") !== "collapsed";

const requestedPane = query.get("pane");

const settingsPane: VisualSettingsPane = isVisualSettingsPane(requestedPane)
  ? requestedPane
  : "appearance";

const requestedOverlay = query.get("overlay");

const shellOverlay: VisualShellOverlay | null = isVisualShellOverlay(requestedOverlay)
  ? requestedOverlay
  : null;

const requestedFixture = query.get("fixture");

const fixture =
  requestedFixture === "agent" ||
  requestedFixture === "workspace" ||
  requestedFixture === "shell" ||
  requestedFixture === "computer" ||
  requestedFixture === "composition" ||
  requestedFixture === "atoms"
    ? requestedFixture
    : "foundation";

const requestedState = query.get("state");

const state: VisualAgentState = VISUAL_AGENT_STATES.includes(requestedState as VisualAgentState)
  ? (requestedState as VisualAgentState)
  : "running";

const workIndexState: VisualWorkIndexState = VISUAL_WORK_INDEX_STATES.includes(
  requestedState as VisualWorkIndexState,
)
  ? (requestedState as VisualWorkIndexState)
  : "populated";

const workspaceState: VisualWorkspaceState = isVisualWorkspaceState(requestedState)
  ? requestedState
  : "dock-review";

const rootElement = document.documentElement;

const motionScale = fixtureMotionScale(query.get("motion"));

function extendRecordedSource(snapshot: RuntimeAgentSessionSnapshot) {
  if (!query.has("long-result")) return;
  const read = snapshot.items.find((item) => item.type === "toolCall" && item.tool.name === "read");
  if (!read || read.type !== "toolCall")
    throw new Error("The fixture needs a read call to show a long result");
  read.tool.result = {
    content: Array.from(
      { length: 82 },
      (_, index) =>
        `Recorded boundary observation ${String(index + 1).padStart(2, "0")} · 中文证据`,
    ).join("\n"),
    start_line: 1,
    end_line: 82,
    total_lines: 82,
  };
}

const requestedFontSize = query.get("font-size");

const requestedDensity = query.get("density");

const density = UI_DENSITY_MODES.find((mode) => mode === requestedDensity);

const colors = fixtureColors(query);

const requestedFullView = query.get("full-view") ?? undefined;

const requestedReviewFiles = Number(query.get("review-files")) || undefined;

const requestedLocale = query.get("locale") ?? FIXTURE_DEFAULT_LOCALE;

const requestedUiFont = query.get("ui-font") ?? undefined;

const requestedSmoothing = query.get("smoothing");

const fontSmoothing = requestedSmoothing === null ? undefined : requestedSmoothing !== "off";

const requestedRadius = Number(query.get("radius"));

const radiusScale =
  Number.isFinite(requestedRadius) && query.get("radius") !== null ? requestedRadius : undefined;

rootElement.classList.remove("theme-light", "theme-dark");

rootElement.classList.add(`theme-${theme}`);

rootElement.style.setProperty("--motion-scale", String(motionScale));

visualClient.theme.appearance.publishMotionScale(motionScale);

if (motionScale === 0) rootElement.dataset.motion = "off";
else delete rootElement.dataset.motion;

if (requestedFontSize !== null && Number.isFinite(Number(requestedFontSize))) {
  for (const [property, value] of Object.entries(
    uiTypeLadderCssVariables(Number(requestedFontSize)),
  )) {
    rootElement.style.setProperty(property, value);
  }
}

rootElement.dataset.visualTheme = theme;

const container = document.getElementById("root");

if (!container) throw new Error("Visual fixture root element is missing");

async function installVisualRuntimeClient(
  snapshot: RuntimeAgentSessionSnapshot,
  capabilities: ServerCapabilities,
  readDiff: (signal?: AbortSignal) => Promise<Diff>,
  acceptSkillProposals = false,
): Promise<FlameClient> {
  const transport = createVisualRuntimeTransport(snapshot, capabilities, readDiff, {
    acceptSkillProposals,
    modelFacts:
      workspaceState === "dock-schedules-models"
        ? SCROLLING_MODEL_FACTS
        : query.has("long-models")
          ? LONG_MODEL_FACTS
          : undefined,
    skillDocuments:
      workspaceState === "dock-skills-catalog" ? VISUAL_LONG_SKILL_DOCUMENTS : undefined,
    memoryPages:
      workspaceState === "dock-agent-memory-recorded"
        ? { project: VISUAL_LONG_MEMORY_PAGE, user: VISUAL_LONG_USER_MEMORY_PAGE }
        : undefined,
    mcpServers: query.has("mcp-transport")
      ? [
          {
            id: {
              origin: { type: "user" },
              name: query.has("long-mcp") ? "workspace-permissions-toolserver" : "workspace-tools",
            },
            connection:
              query.get("mcp-transport") === "streamableHttp"
                ? {
                    type: "streamableHttp",
                    url: "https://example.invalid/mcp",
                    authorizationMasked: "synthetic-mask",
                  }
                : { type: "stdio", command: "npx", args: [] },
            handshakeTimeout: { type: "bounded", seconds: 30 },
            status: query.has("long-mcp")
              ? { type: "failed", error: { type: "mcp_configuration_failed" } }
              : { type: "connected", toolCount: 1 },
          },
        ]
      : undefined,
  });
  const client = createFlameClient(transport, { capabilities: () => capabilities });
  await loadPluginsForTest(
    definePlugin({
      name: "flame.visual.runtime-client",
      setup(ctx) {
        const removeSchedule = (event: Event) => {
          if (!(event instanceof CustomEvent) || typeof event.detail !== "string")
            throw new Error("visual schedule removal requires a record identity");
          transport.removeSchedule(event.detail);
        };
        window.addEventListener(VISUAL_REMOVE_SCHEDULE_EVENT, removeSchedule);
        ctx.lifetime.defer(() => {
          window.removeEventListener(VISUAL_REMOVE_SCHEDULE_EVENT, removeSchedule);
          return client.close();
        });
      },
    }),
  );
  currentRuntimeClient = client;
  return client;
}

async function fixtureNode(): Promise<ReactNode> {
  if (
    fixture === "foundation" ||
    fixture === "atoms" ||
    fixture === "computer" ||
    fixture === "composition"
  ) {
    const [
      { default: flameLight },
      { default: flameDark },
      { builtinVisualStyles },
      { defaultAccents },
    ] = await Promise.all([
      import("@/features/theme/themes/flame-light"),
      import("@/features/theme/themes/flame-dark"),
      import("@/features/theme/visualStyles/index"),
      import("@/features/theme/defaultAccents"),
    ]);
    for (const plugin of [
      flameLight(visualClient.theme.access.preference),
      flameDark(visualClient.theme.access.preference),
      defaultAccents,
      ...builtinVisualStyles,
    ]) {
      await loadPluginsForTest(plugin);
    }
    if (fixture === "composition") {
      const { createCompositionFixture } = await import("./compositionBaseline");
      const composition = await createCompositionFixture(requestedState ?? "one", (children) => (
        <FixturePresentation>{children}</FixturePresentation>
      ));
      currentRuntimeClient = composition.client;
      return composition.node;
    }
    if (fixture === "computer") {
      const { createComputerConsoleFixture } = await import("./computerConsoleFixture");
      const computer = await createComputerConsoleFixture(requestedLocale, requestedState);
      currentRuntimeClient = computer.client;
      await loadPluginsForTest(
        definePlugin({
          name: "flame.visual.computer",
          setup(ctx) {
            ctx.lifetime.defer(computer.close);
          },
        }),
      );
      return computer.node;
    }
    if (fixture === "atoms") {
      const { VisualAtomsFixture, VisualInputContractFixture } =
        await import("./VisualAtomsFixture");
      return requestedState === "input-contracts" ? (
        <VisualInputContractFixture />
      ) : (
        <VisualAtomsFixture />
      );
    }
    return <VisualFoundationFixture sidebarOpen={sidebarOpen} />;
  }
  if (fixture === "workspace") {
    const [{ VisualWorkspaceFixture }, { installVisualWorkspaceFixture, visualWorkspaceRuntime }] =
      await Promise.all([
        import("./VisualWorkspaceFixture"),
        import("./installVisualWorkspaceFixture"),
      ]);
    const runtimeInput = visualWorkspaceRuntime(
      workspaceState,
      VISUAL_AGENT_STATES.includes(query.get("agent-state") as VisualAgentState)
        ? (query.get("agent-state") as VisualAgentState)
        : undefined,
      requestedReviewFiles,
      query.get("review-path") ?? undefined,
    );
    extendRecordedSource(runtimeInput.snapshot);
    const runtimeClient = await installVisualRuntimeClient(
      runtimeInput.snapshot,
      runtimeInput.capabilities,
      runtimeInput.readDiff,
      query.get("skills-commands") === "accept",
    );
    await installVisualWorkspaceFixture(
      visualClient,
      () => runtimeClient,
      workspaceState,
      theme,
      runtimeInput,
      {
        pane: settingsPane,
        fullViewId: requestedFullView,
      },
    );
    return <VisualWorkspaceFixture state={workspaceState} />;
  }
  if (fixture === "shell") {
    const [{ VisualShellFixture }, { installVisualShellFixture }] = await Promise.all([
      import("./VisualShellFixture"),
      import("./installVisualShellFixture"),
    ]);
    const runtimeClient = await installVisualRuntimeClient(
      structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS.idle),
      VISUAL_CAPABILITIES,
      async () => {
        throw new Error("The shell fixture has no workspace diff read");
      },
    );
    await installVisualShellFixture(
      visualClient,
      () => runtimeClient,
      workIndexState,
      theme,
      sidebarOpen,
      shellOverlay,
    );
    return <VisualShellFixture state={workIndexState} />;
  }
  const [{ VisualAgentStateFixture }, { installVisualAgentFixture }] = await Promise.all([
    import("./VisualAgentStateFixture"),
    import("./installVisualAgentFixture"),
  ]);
  const output = query.has("long-output")
    ? Array.from({ length: 50_000 }, (_, index) =>
        index === 31_415
          ? "build output li\u001b[31mne 31415\u001b[0m"
          : `build output line ${index}`,
      ).join("\n")
    : undefined;
  if (query.has("resume-ready") && state !== "delegated")
    throw new Error("Only the delegated fixture has a parked approval tree");
  const snapshot = query.has("resume-ready")
    ? delegatedApprovalSnapshot()
    : structuredClone(RUNTIME_AGENT_SESSION_SNAPSHOTS[state]);
  extendRecordedSource(snapshot);
  const runtimeClient = await installVisualRuntimeClient(
    snapshot,
    VISUAL_CAPABILITIES,
    async () => {
      throw new Error("The Agent fixture has no workspace diff read");
    },
  );
  const view = await installVisualAgentFixture(
    visualClient,
    () => runtimeClient,
    state,
    output,
    snapshot,
    VISUAL_CAPABILITIES,
    fileListState,
  );
  visualClient.workspace.layout.setState({
    sidebarCollapsed: !sidebarOpen,
    sidebarWidth: SIDEBAR_DEFAULT_WIDTH_PX,
  });
  return <VisualAgentStateFixture state={state} view={view} />;
}

const { i18nPlugin } = await import("@/features/i18n/index");
const { visualLocalePackages } = await import("./localePackages");

await loadPluginsForTest(i18nPlugin, visualLocalePackages);

const locale = lookupExtensionByKey(LOCALE, requestedLocale);

if (!locale) throw new Error(`Visual locale is not registered: ${requestedLocale}`);

await selectLocale(locale);

const node = await fixtureNode();

visualClient.theme.store.setState({
  theme,
  motionScale,
  ...(requestedFontSize !== null && Number.isFinite(Number(requestedFontSize))
    ? { fontSize: Number(requestedFontSize) }
    : {}),
  ...(density ? { density } : {}),
  ...colors,
  ...(requestedUiFont ? { uiFont: requestedUiFont } : {}),
  ...(radiusScale !== undefined ? { radiusScale } : {}),
  ...(fontSmoothing !== undefined ? { fontSmoothing } : {}),
});

visualClient.theme.start();

const root = createRoot(container);
function FixturePresentation({ children }: { children: ReactNode }) {
  return (
    <visualClient.Wrapper>
      <QueryClientProvider client={queryClient}>
        <MotionConfig reducedMotion="user">
          <FontScaleProvider fontSize={visualClient.theme.store.getState().fontSize ?? 14}>
            <CodeHighlightProvider component={HighlightedCodeSource}>
              <TooltipProvider>{children}</TooltipProvider>
            </CodeHighlightProvider>
          </FontScaleProvider>
        </MotionConfig>
      </QueryClientProvider>
    </visualClient.Wrapper>
  );
}

root.render(<FixturePresentation>{node}</FixturePresentation>);

const dispose = () => {
  root.unmount();
  void visualClient.dispose();
  restoreFixtureClock();
};
window.addEventListener("beforeunload", dispose);
disposeOnHmr(() => {
  window.removeEventListener("beforeunload", dispose);
  dispose();
});

const FIXTURE_FACES = ['1rem "JetBrains Mono"'];

function shellArrived(deadlineMs: number): Promise<void> {
  return new Promise((resolve, reject) => {
    const expiry = performance.now() + deadlineMs;
    const check = () => {
      const pending = rootElement.querySelector("[data-surface-pending]");
      if (container!.firstElementChild && !pending) resolve();
      else if (performance.now() > expiry)
        reject(new Error("Fixture surfaces did not arrive before the readiness deadline"));
      else requestAnimationFrame(check);
    };
    check();
  });
}

const ARRIVAL_DEADLINE_MS = 8000;

function renderedFrame(): Promise<void> {
  return new Promise((resolve) =>
    requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
  );
}

// Arrived surfaces request their own faces, such as KaTeX's, only once laid out.
void Promise.all(FIXTURE_FACES.map((face) => document.fonts.load(face)))
  .then(() => shellArrived(ARRIVAL_DEADLINE_MS))
  .then(renderedFrame)
  .then(() => document.fonts.ready)
  .then(renderedFrame)
  .then(async () => {
    if (fixture === "agent" && fileListState !== undefined) {
      const input = container.querySelector("textarea");
      if (!input) throw new Error("file suggestions require the mounted Composer");
      input.setSelectionRange(VISUAL_FILE_MENTION_DRAFT.length, VISUAL_FILE_MENTION_DRAFT.length);
      await renderedFrame();
      visualClient.composer.access.focus(VISUAL_FILE_MENTION_DRAFT.length);
    }
    rootElement.dataset.visualReady = "";
  });
