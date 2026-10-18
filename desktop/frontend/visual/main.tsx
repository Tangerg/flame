import { createFlameClient, type FlameClient } from "@flame/runtime-contract/client";
import { VISUAL_CAPABILITIES } from "./agentFixtureFacts";
import type { ServerCapabilities, Diff } from "@flame/runtime-contract/wire";
import { createVisualRuntimeTransport } from "./createVisualRuntimeTransport";
import { definePlugin } from "@/plugins/sdk";
import { createRoot } from "react-dom/client";
import type { ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { MotionConfig } from "motion/react";
import "@flame/ui/styles.css";
import { TooltipProvider, FontScaleProvider } from "@flame/ui";
import { publishMotionScale } from "@/lib/appearance";
import { queryClient } from "@/lib/queryClient";
import { LOCALE, lookupExtensionByKey } from "@/plugins/sdk";
import { selectLocale } from "@/plugins/builtin/settings/appearance/application/localeSelection";
import { configureNavigator } from "@/lib/navigation";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import { uiTypeLadderCssVariables } from "@/plugins/builtin/theme/kit/typeLadder";
import { UI_DENSITY_MODES } from "@/plugins/builtin/theme/kit/appearance";
import { installAppearancePreferencePort } from "@/plugins/builtin/theme/adapters/appearancePreferenceBinding";
import { installDocumentAppearance } from "@/plugins/builtin/theme/adapters/documentAppearance";
import { useAppearanceStore } from "@/plugins/builtin/theme/adapters/appearanceStore";
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
import "../src/styles/markdown.css";
import "../src/styles/overlays.css";
import "../src/styles/globals.css";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { VISUAL_NOW } from "./agentFixtureFacts";

type FixtureTheme = "light" | "dark";

const VISUAL_CLOCK_STARTED_AT = performance.now();
Date.now = () => VISUAL_NOW + (performance.now() - VISUAL_CLOCK_STARTED_AT);
configureNavigator(createMemoryNavigator());

function fixtureTheme(value: string | null): FixtureTheme {
  return value === "dark" ? "dark" : "light";
}

const query = new URLSearchParams(window.location.search);
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
const motionScale = query.get("motion") === "full" ? 1 : 0;
const requestedFontSize = query.get("font-size");
const requestedDensity = query.get("density");
const density = UI_DENSITY_MODES.find((mode) => mode === requestedDensity);
const requestedAccent = query.get("accent");
const accent =
  requestedAccent !== null && /^#[\da-f]{6}$/i.test(requestedAccent) ? requestedAccent : undefined;
const requestedContrast = Number(query.get("contrast"));
const contrast =
  Number.isFinite(requestedContrast) && query.get("contrast") !== null
    ? Math.min(100, Math.max(0, requestedContrast))
    : undefined;
const requestedFullView = query.get("full-view") ?? undefined;
const requestedReviewFiles = Number(query.get("review-files")) || undefined;
const requestedLocale = query.get("locale") ?? "en";
const hex = (name: string) => {
  const value = query.get(name);
  return value !== null && /^#[\da-f]{6}$/i.test(value) ? value : undefined;
};
const requestedUiFont = query.get("ui-font") ?? undefined;
const requestedSmoothing = query.get("smoothing");
const fontSmoothing = requestedSmoothing === null ? undefined : requestedSmoothing !== "off";
const requestedRadius = Number(query.get("radius"));
const radiusScale =
  Number.isFinite(requestedRadius) && query.get("radius") !== null ? requestedRadius : undefined;
const customBase = hex("custom-bg");
const customInk = hex("custom-fg");
const customTheme =
  customBase !== undefined && customInk !== undefined
    ? { bg: customBase, fg: customInk }
    : undefined;

rootElement.classList.remove("theme-light", "theme-dark");
rootElement.classList.add(`theme-${theme}`);
rootElement.style.setProperty("--motion-scale", String(motionScale));
publishMotionScale(motionScale);
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
  const client = createFlameClient(
    createVisualRuntimeTransport(snapshot, capabilities, readDiff, acceptSkillProposals),
    { capabilities: () => capabilities },
  );
  await loadPluginsForTest(
    definePlugin({
      name: "flame.visual.runtime-client",
      setup(ctx) {
        ctx.cleanup(() => client.close());
      },
    }),
  );
  return client;
}

async function fixtureNode(): Promise<ReactNode> {
  if (fixture === "foundation" || fixture === "atoms") {
    const [
      { default: flameLight },
      { default: flameDark },
      { builtinVisualStyles },
      { defaultAccents },
    ] = await Promise.all([
      import("@/plugins/builtin/theme/themes/flame-light"),
      import("@/plugins/builtin/theme/themes/flame-dark"),
      import("@/plugins/builtin/theme/visualStyles"),
      import("@/plugins/builtin/defaults"),
    ]);
    for (const plugin of [flameLight, flameDark, defaultAccents, ...builtinVisualStyles]) {
      await loadPluginsForTest(plugin);
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
    const runtimeClient = await installVisualRuntimeClient(
      runtimeInput.snapshot,
      runtimeInput.capabilities,
      runtimeInput.readDiff,
      query.get("skills-commands") === "accept",
    );
    await installVisualWorkspaceFixture(() => runtimeClient, workspaceState, theme, runtimeInput, {
      pane: settingsPane,
      fullViewId: requestedFullView,
    });
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
  const runtimeClient = await installVisualRuntimeClient(
    snapshot,
    VISUAL_CAPABILITIES,
    async () => {
      throw new Error("The Agent fixture has no workspace diff read");
    },
  );
  const view = await installVisualAgentFixture(
    () => runtimeClient,
    state,
    output,
    snapshot,
    VISUAL_CAPABILITIES,
  );
  return <VisualAgentStateFixture state={state} view={view} />;
}

const { localePlugins } = await import("@/plugins/builtin/i18n");
await loadPluginsForTest(...localePlugins);
const locale = lookupExtensionByKey(LOCALE, requestedLocale);
if (!locale) throw new Error(`Visual locale is not registered: ${requestedLocale}`);
await selectLocale(locale);

const node = await fixtureNode();

useAppearanceStore.setState({
  theme,
  motionScale,
  ...(requestedFontSize !== null && Number.isFinite(Number(requestedFontSize))
    ? { fontSize: Number(requestedFontSize) }
    : {}),
  ...(density ? { density } : {}),
  ...(contrast !== undefined ? { contrast } : {}),
  ...(accent !== undefined ? { accent } : {}),
  ...(customTheme ? { theme: "custom" as const, customTheme } : {}),
  ...(requestedUiFont ? { uiFont: requestedUiFont } : {}),
  ...(radiusScale !== undefined ? { radiusScale } : {}),
  ...(fontSmoothing !== undefined ? { fontSmoothing } : {}),
});
installAppearancePreferencePort();
installDocumentAppearance(useAppearanceStore);

createRoot(container).render(
  <QueryClientProvider client={queryClient}>
    <MotionConfig reducedMotion="user">
      <FontScaleProvider fontSize={useAppearanceStore.getState().fontSize ?? 14}>
        <TooltipProvider>{node}</TooltipProvider>
      </FontScaleProvider>
    </MotionConfig>
  </QueryClientProvider>,
);

const FIXTURE_FACES = ['1rem "JetBrains Mono"'];

function shellArrived(deadlineMs: number): Promise<void> {
  return new Promise((resolve) => {
    const expiry = performance.now() + deadlineMs;
    const check = () => {
      const pending = rootElement.querySelector("[data-surface-pending]");
      if (!pending || performance.now() > expiry) resolve();
      else requestAnimationFrame(check);
    };
    check();
  });
}

const ARRIVAL_DEADLINE_MS = 8000;

void Promise.all(FIXTURE_FACES.map((face) => document.fonts.load(face)))
  .then(() => document.fonts.ready)
  .then(() => shellArrived(ARRIVAL_DEADLINE_MS))
  .then(
    () =>
      new Promise<void>((resolve) =>
        requestAnimationFrame(() => requestAnimationFrame(() => resolve())),
      ),
  )
  .then(() => {
    rootElement.dataset.visualReady = "";
  });
