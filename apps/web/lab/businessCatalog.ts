import { COMPOSITION_SAMPLES } from "./scenarios/compositionFixtureStates";
import { COMPUTER_FIXTURE_VARIANTS } from "./scenarios/computerFixtureStates";
import { VISUAL_AGENT_STATES } from "./scenarios/agentFixtureStates";
import { VISUAL_WORK_INDEX_STATES } from "./scenarios/shellFixtureStates";
import { VISUAL_SETTINGS_PANES, VISUAL_WORKSPACE_STATES } from "./scenarios/workspaceFixtureStates";
import { businessMessages as messages } from "./businessMessages";
import {
  activityVariants,
  attachmentVariants,
  catalogVariants,
  composerVariants,
  dataVariants,
  fileVariants,
  messageVariants,
  navigationVariants,
  referenceVariants,
  reasoningVariants,
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

function compositionVariant(sample: (typeof COMPOSITION_SAMPLES)[number]): FixtureVariant {
  return {
    id: sample.id,
    title: {
      en: sample.frames === 1 ? "1 frame" : `${sample.frames} frames`,
      "zh-CN": `${sample.frames} 层框架`,
    },
    target: { fixture: "composition", state: sample.id },
  };
}

function computerVariant(sample: (typeof COMPUTER_FIXTURE_VARIANTS)[number]): FixtureVariant {
  return { ...sample, target: { fixture: "computer", state: sample.id } };
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
    id: "business-references",
    ...messages.stories.references,
    group: messages.groups.conversation,
    source: "apps/web/src/features/chat/message/ui/markdown/MarkdownRenderer.tsx",
    width: 560,
    height: 640,
    variants: referenceVariants,
    load: () =>
      import("./businessReferences").then((module) => ({ default: module.ReferencesExample })),
  },
  {
    kind: "component",
    panel: "business",
    id: "business-reasoning",
    ...messages.stories.reasoning,
    group: messages.groups.conversation,
    source: "apps/web/src/features/chat/message/ui/cards/ReasoningBlock.tsx",
    ...READING_PREVIEW,
    variants: reasoningVariants,
    load: () =>
      import("./businessReasoning").then((module) => ({ default: module.ReasoningExample })),
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
    id: "business-attachments",
    ...messages.stories.attachments,
    group: messages.groups.conversation,
    source: "apps/web/src/features/composer/ui/ComposerAttachments.tsx",
    width: 320,
    height: 360,
    variants: attachmentVariants,
    load: () =>
      import("./businessAttachments").then((module) => ({ default: module.AttachmentsExample })),
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
    source: "apps/web/lab/scenarios/installVisualAgentFixture.ts",
    ...PRODUCT_PREVIEW,
    variants: fixtureVariants(VISUAL_AGENT_STATES, (state) => ({ fixture: "agent", state })),
  },
  {
    kind: "fixture",
    panel: "business",
    id: "business-workspace-scenarios",
    ...messages.stories.workspace,
    group: messages.groups.scenarios,
    source: "apps/web/lab/scenarios/installVisualWorkspaceFixture.ts",
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
    source: "apps/web/lab/scenarios/installVisualShellFixture.ts",
    ...PRODUCT_PREVIEW,
    variants: fixtureVariants(VISUAL_WORK_INDEX_STATES, (state) => ({ fixture: "shell", state })),
  },
  {
    kind: "fixture",
    panel: "business",
    id: "business-computer",
    title: { en: "Computer console", "zh-CN": "计算机控制台" },
    summary: {
      en: "Production console over recorded surfaces and an in-memory transport.",
      "zh-CN": "使用记录的画面和内存传输展示生产控制台。",
    },
    group: messages.groups.scenarios,
    source: "apps/web/lab/scenarios/computerConsoleFixture.tsx",
    ...PRODUCT_PREVIEW,
    variants: [
      computerVariant(COMPUTER_FIXTURE_VARIANTS[0]),
      ...COMPUTER_FIXTURE_VARIANTS.slice(1).map(computerVariant),
    ],
  },
  {
    kind: "fixture",
    panel: "business",
    id: "business-composition",
    title: { en: "Installed composition", "zh-CN": "安装组件组合" },
    summary: {
      en: "Production module loading and decoration rendering over admitted fixture bindings.",
      "zh-CN": "使用已准入的测试绑定，检查生产模块加载和装饰组合。",
    },
    group: messages.groups.scenarios,
    source: "apps/web/lab/scenarios/compositionBaseline.tsx",
    ...PRODUCT_PREVIEW,
    variants: [
      compositionVariant(COMPOSITION_SAMPLES[0]),
      ...COMPOSITION_SAMPLES.slice(1).map(compositionVariant),
    ],
  },
];
