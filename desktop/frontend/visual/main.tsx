import { createRoot } from "react-dom/client";
import type { ReactNode } from "react";
import { QueryClientProvider } from "@tanstack/react-query";
import { MotionConfig } from "motion/react";
import { TooltipProvider } from "@/ui";
import { publishMotionScale } from "@/lib/appearance";
import { queryClient } from "@/lib/queryClient";
import { setLocale, t } from "@/lib/i18n";
import { configureNavigator } from "@/lib/navigation";
import { createMemoryNavigator } from "@/lib/navigation.testkit";
import { uiTypeLadderCssVariables } from "@/plugins/builtin/theme/kit/typeLadder";
import { UI_DENSITY_MODES } from "@/plugins/builtin/theme/kit/appearance";
import { installAppearancePreferencePort } from "@/plugins/builtin/theme/adapters/appearancePreferenceBinding";
import { installDocumentAppearance } from "@/plugins/builtin/theme/adapters/documentAppearance";
import { useAppearanceStore } from "@/plugins/builtin/theme/adapters/appearanceStore";
import { VisualFoundationFixture } from "./VisualFoundationFixture";
import { VISUAL_AGENT_STATES, type VisualAgentState } from "./agentSessionSnapshots";
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
import "../src/styles/stylex.css";
import { loadPluginsForTest } from "@/plugins/sdk/testKernel";
import { VISUAL_NOW } from "./agentFixtureFacts";

type FixtureTheme = "light" | "dark";

const VISUAL_CLOCK_STARTED_AT = performance.now();
Date.now = () => VISUAL_NOW + (performance.now() - VISUAL_CLOCK_STARTED_AT);
setLocale("en");
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
  requestedFixture === "agent" || requestedFixture === "workspace" || requestedFixture === "shell"
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

async function fixtureNode(): Promise<ReactNode> {
  if (fixture === "foundation") {
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
    return <VisualFoundationFixture sidebarOpen={sidebarOpen} />;
  }
  if (fixture === "workspace") {
    const [{ VisualWorkspaceFixture }, { installVisualWorkspaceFixture }] = await Promise.all([
      import("./VisualWorkspaceFixture"),
      import("./installVisualWorkspaceFixture"),
    ]);
    await installVisualWorkspaceFixture(workspaceState, theme, settingsPane, requestedFullView);
    return <VisualWorkspaceFixture state={workspaceState} />;
  }
  if (fixture === "shell") {
    const [{ VisualShellFixture }, { installVisualShellFixture }] = await Promise.all([
      import("./VisualShellFixture"),
      import("./installVisualShellFixture"),
    ]);
    await installVisualShellFixture(workIndexState, theme, sidebarOpen, shellOverlay);
    return <VisualShellFixture state={workIndexState} />;
  }
  const [{ VisualAgentStateFixture }, { installVisualAgentFixture }] = await Promise.all([
    import("./VisualAgentStateFixture"),
    import("./installVisualAgentFixture"),
  ]);
  const view = await installVisualAgentFixture(state);
  return <VisualAgentStateFixture state={state} view={view} />;
}

if (requestedLocale !== "en") {
  setLocale(requestedLocale);
  const [{ localePlugins }, { loadPluginsForTest }] = await Promise.all([
    import("@/plugins/builtin/i18n"),
    import("@/plugins/sdk/testKernel"),
  ]);
  for (const plugin of localePlugins) await loadPluginsForTest(plugin);
  const { en } = await import("@/lib/i18n/locales/en");
  for (let attempt = 0; attempt < 400 && t("common.cancel") === en["common.cancel"]; attempt += 1) {
    await new Promise((resolve) => setTimeout(resolve, 5));
  }
}

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
      <TooltipProvider>{node}</TooltipProvider>
    </MotionConfig>
  </QueryClientProvider>,
);

const FIXTURE_FACES = ['1rem "Geist"', '1rem "JetBrains Mono"'];

function shellArrived(deadlineMs: number): Promise<void> {
  return new Promise((resolve) => {
    const expiry = performance.now() + deadlineMs;
    const check = () => {
      const pending = rootElement.querySelector("[data-workspace-view-pending]");
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
