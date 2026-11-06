import { VISUAL_AGENT_STATES } from "../agentSessionSnapshots";
import { VISUAL_WORK_INDEX_STATES } from "../shellFixtureStates";
import { VISUAL_SETTINGS_PANES, VISUAL_WORKSPACE_STATES } from "../workspaceFixtureStates";
import { businessMessages as messages } from "./businessMessages";
import {
  activityVariants,
  catalogVariants,
  composerVariants,
  dataVariants,
  fileVariants,
  messageVariants,
  navigationVariants,
  statusVariants,
} from "./businessVariants";
import type { FixtureVariant, LabFixtureTarget, LabStory } from "./story";

const COMPACT_PREVIEW = { width: 480, height: 260 } as const;
const READING_PREVIEW = { width: 560, height: 360 } as const;
const CATALOG_PREVIEW = { width: 480, height: 420 } as const;
const PRODUCT_PREVIEW = { width: 1440, height: 900 } as const;

function fixtureVariants<T extends string>(
  values: readonly [T, ...T[]],
  target: (value: T) => LabFixtureTarget,
): readonly [FixtureVariant, ...FixtureVariant[]] {
  const variant = (value: T): FixtureVariant => ({
    id: value,
    title: { "zh-CN": value, en: value },
    target: target(value),
  });
  return [variant(values[0]), ...values.slice(1).map(variant)];
}

export const businessStories: readonly LabStory[] = [
  {
    kind: "component",
    panel: "business",
    id: "business-status",
    ...messages.stories.status,
    group: messages.groups.conversation,
    source: "apps/web/src/ui/agent/status-pill.tsx",
    ...COMPACT_PREVIEW,
    variants: statusVariants,
    load: () =>
      import("./businessNavigation").then((module) => ({ default: module.StatusExample })),
  },
  {
    kind: "component",
    panel: "business",
    id: "business-navigation",
    ...messages.stories.navigation,
    group: messages.groups.conversation,
    source: "apps/web/src/ui/agent/navigation-row.tsx",
    ...READING_PREVIEW,
    variants: navigationVariants,
    load: () =>
      import("./businessNavigation").then((module) => ({ default: module.NavigationExample })),
  },
  {
    kind: "component",
    panel: "business",
    id: "business-activity",
    ...messages.stories.activity,
    group: messages.groups.conversation,
    source: "apps/web/src/ui/agent/activity-disclosure.tsx",
    ...READING_PREVIEW,
    variants: activityVariants,
    load: () =>
      import("./businessActivity").then((module) => ({ default: module.ActivityExample })),
  },
  {
    kind: "component",
    panel: "business",
    id: "business-composer",
    ...messages.stories.composer,
    group: messages.groups.conversation,
    source: "apps/web/src/ui/agent/composer-chip.tsx",
    ...READING_PREVIEW,
    variants: composerVariants,
    load: () =>
      import("./businessComposer").then((module) => ({ default: module.ComposerChipExample })),
  },
  {
    kind: "component",
    panel: "business",
    id: "business-catalog",
    ...messages.stories.catalog,
    group: messages.groups.workspace,
    source: "apps/web/src/ui/agent/dock-catalog.tsx",
    ...CATALOG_PREVIEW,
    variants: catalogVariants,
    load: () =>
      import("./businessCatalogPreview").then((module) => ({ default: module.CatalogExample })),
  },
  {
    kind: "component",
    panel: "business",
    id: "business-data",
    ...messages.stories.data,
    group: messages.groups.workspace,
    source: "apps/web/src/ui/atoms/data-view.tsx",
    ...READING_PREVIEW,
    variants: [dataVariants.ready, dataVariants.loading, dataVariants.empty, dataVariants.failure],
    load: () => import("./businessFeedback").then((module) => ({ default: module.DataExample })),
  },
  {
    kind: "component",
    panel: "business",
    id: "business-message",
    ...messages.stories.message,
    group: messages.groups.workspace,
    source: "apps/web/src/ui/atoms/system-message.tsx",
    ...COMPACT_PREVIEW,
    variants: messageVariants,
    load: () => import("./businessFeedback").then((module) => ({ default: module.MessageExample })),
  },
  {
    kind: "component",
    panel: "business",
    id: "business-files",
    ...messages.stories.files,
    group: messages.groups.workspace,
    source: "apps/web/src/ui/atoms/file-path.tsx",
    ...READING_PREVIEW,
    variants: fileVariants,
    load: () =>
      import("./businessFiles").then((module) => ({ default: module.FileIdentityExample })),
  },
  {
    kind: "fixture",
    panel: "business",
    id: "business-agent-scenarios",
    ...messages.stories.agent,
    group: messages.groups.scenarios,
    source: "apps/web/visual/installVisualAgentFixture.ts",
    ...PRODUCT_PREVIEW,
    variants: fixtureVariants(VISUAL_AGENT_STATES, (state) => ({ fixture: "agent", state })),
  },
  {
    kind: "fixture",
    panel: "business",
    id: "business-workspace-scenarios",
    ...messages.stories.workspace,
    group: messages.groups.scenarios,
    source: "apps/web/visual/installVisualWorkspaceFixture.ts",
    ...PRODUCT_PREVIEW,
    variants: fixtureVariants(VISUAL_WORKSPACE_STATES, (state) => ({
      fixture: "workspace",
      state,
    })),
  },
  {
    kind: "fixture",
    panel: "business",
    id: "business-settings-scenarios",
    ...messages.stories.settings,
    group: messages.groups.scenarios,
    source: "apps/web/src/features/settings/public/SettingsPage.tsx",
    ...PRODUCT_PREVIEW,
    variants: fixtureVariants(VISUAL_SETTINGS_PANES, (pane) => ({
      fixture: "workspace",
      state: "settings",
      pane,
    })),
  },
  {
    kind: "fixture",
    panel: "business",
    id: "business-shell-scenarios",
    ...messages.stories.shell,
    group: messages.groups.scenarios,
    source: "apps/web/visual/installVisualShellFixture.ts",
    ...PRODUCT_PREVIEW,
    variants: fixtureVariants(VISUAL_WORK_INDEX_STATES, (state) => ({ fixture: "shell", state })),
  },
];
