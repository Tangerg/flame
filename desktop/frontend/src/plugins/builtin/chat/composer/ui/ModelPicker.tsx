import { useMemo, useState } from "react";
import * as stylex from "@stylexjs/stylex";

import { fmtTokens } from "@/lib/format";
import { type Translate, useT } from "@/lib/i18n";
import { reasoningEffortLabel } from "@/plugins/builtin/settings/providers/public/reasoningEffort";
import { radius, space, surface, type as typeStep, weight } from "@/styles/tokens.stylex";
import {
  Button,
  Icon,
  Popover,
  StepSlider,
  vocab,
  ProviderIcon,
  RailCatalogPicker,
  SkeletonControl,
  type CatalogPickerGroup,
  providerDisplayName,
} from "@/ui";
import {
  type SelectableModel,
  useModels,
} from "@/plugins/builtin/settings/providers/public/queries";
import { useRecentModelsStore, type RecentModel } from "../adapters/recentModels";
import { AgentComposerChip } from "@/ui/agent";
import { useSetComposerModelPreference } from "../public/modelPreference";
import { useSelectedModelSelection } from "../public/selectedModel";

type Selection = NonNullable<ReturnType<typeof useSelectedModelSelection>>;

const effortStyles = stylex.create({
  panel: {
    display: "flex",
    width: "260px",
    flexDirection: "column",
    gap: space.s2,
    padding: space.s3,
  },
  heading: { display: "flex", gap: space.s1 },
  value: { fontWeight: weight.medium },
  ends: { display: "flex", justifyContent: "space-between" },
  fixed: {
    display: "inline-flex",
    height: "var(--control-height-xs)",
    alignItems: "center",
    borderRadius: radius.button,
    paddingInline: space.s2,
    backgroundColor: surface.hover,
    whiteSpace: "nowrap",
  },
});

function EffortChip({ selection }: { selection: Selection }) {
  const t = useT();
  const setModel = useSetComposerModelPreference();
  const { model } = selection;
  const levels = model.reasoningLevels;
  const committed = model.reasoningLevelOrDefault(selection.reasoningEffort)!;
  const [preview, setPreview] = useState<string | null>(null);
  const shown = preview ?? committed;
  const commit = (index: number) => {
    setPreview(null);
    const effort = levels[index];
    if (effort === undefined || effort === committed) return;
    setModel({
      kind: "explicit",
      provider: model.provider,
      model: model.id,
      reasoningEffort: effort,
    });
  };
  return (
    <Popover.Root onOpenChange={(open) => !open && setPreview(null)}>
      <Popover.Trigger
        render={
          <Button
            variant="soft"
            size="xs"
            aria-label={t("composer.switchReasoningEffort")}
            title={t("composer.reasoningEffort")}
          >
            {reasoningEffortLabel(committed, t)}
            <Icon name="chevron-down" size="xs" className={stylex.props(vocab.faint).className} />
          </Button>
        }
      />
      <Popover.Content side="right" align="start" sideOffset={12}>
        <div {...stylex.props(effortStyles.panel)}>
          <div {...stylex.props(effortStyles.heading, typeStep.uiMd)}>
            <span {...stylex.props(vocab.muted)}>{t("composer.effort.title")}</span>
            <span {...stylex.props(effortStyles.value)}>{reasoningEffortLabel(shown, t)}</span>
          </div>
          <div {...stylex.props(effortStyles.ends, vocab.muted, typeStep.uiSm)}>
            <span>{t("composer.effort.faster")}</span>
            <span>{t("composer.effort.smarter")}</span>
          </div>
          <StepSlider
            stops={levels.map((level) => reasoningEffortLabel(level, t))}
            value={levels.indexOf(shown)}
            onValueChange={(index) => setPreview(levels[index] ?? null)}
            onValueCommitted={commit}
            ariaLabel={t("composer.reasoningEffort")}
          />
        </div>
      </Popover.Content>
    </Popover.Root>
  );
}

function effortSuffixed(
  label: string,
  selection: Selection | null | undefined,
  t: Translate,
): string {
  if (!selection || selection.model.reasoningLevels.length === 0) return label;
  const effort = selection.model.reasoningLevelOrDefault(selection.reasoningEffort);
  return effort ? `${label} · ${reasoningEffortLabel(effort, t)}` : label;
}

function modelItemId(model: SelectableModel): string {
  return JSON.stringify([model.provider, model.id]);
}

const RECENT_GROUP_ID = "__recent";

function modelItem(model: SelectableModel, selection: Selection, t: Translate) {
  const active = model.provider === selection.model.provider && model.id === selection.model.id;
  return {
    id: modelItemId(model),
    label: model.label,
    caption: providerDisplayName(model.provider),
    title: modelCapabilities(model, t) || undefined,
    leading: <ProviderIcon provider={model.provider} size="md" />,
    keywords: [model.provider, model.id],
    active,
  };
}

function modelGroups(
  models: readonly SelectableModel[],
  selection: Selection,
  recent: readonly RecentModel[],
  t: Translate,
): CatalogPickerGroup[] {
  const byProvider = new Map<string, SelectableModel[]>();
  for (const model of models) {
    const items = byProvider.get(model.provider);
    if (items) items.push(model);
    else byProvider.set(model.provider, [model]);
  }

  const shelf = recent
    .map((entry) =>
      models.find((model) => model.provider === entry.provider && model.id === entry.id),
    )
    .filter((model): model is SelectableModel => model !== undefined);

  const providers = [...byProvider].map(([provider, items]) => ({
    id: provider,
    label: providerDisplayName(provider),
    leading: <ProviderIcon provider={provider} size="md" />,
    count: items.length,
    items: items.map((model) => modelItem(model, selection, t)),
  }));

  return shelf.length > 0
    ? [
        {
          id: RECENT_GROUP_ID,
          label: t("composer.model.recent"),
          leading: <Icon name="history" size="md" />,
          count: shelf.length,
          items: shelf.map((model) => modelItem(model, selection, t)),
        },
        ...providers,
      ]
    : providers;
}

function modelCapabilities(model: SelectableModel, t: Translate): string {
  const tokenLimits = model.tokenLimits;
  return [
    tokenLimits?.contextWindow !== undefined
      ? t("composer.model.contextWindow", { tokens: fmtTokens(tokenLimits.contextWindow) })
      : null,
    model.inputModalities.length > 0
      ? t("composer.model.inputModalities", { modalities: model.inputModalities.join(" + ") })
      : null,
    model.reasoning
      ? model.reasoningLevels.length > 0
        ? t("composer.model.reasoningLevels", { levels: model.reasoningLevels.join(" / ") })
        : t("composer.model.reasoning")
      : null,
    tokenLimits?.maxInputTokens !== undefined &&
    tokenLimits.maxInputTokens !== tokenLimits.contextWindow
      ? t("composer.model.maxInput", { tokens: fmtTokens(tokenLimits.maxInputTokens) })
      : null,
    tokenLimits?.maxOutputTokens !== undefined
      ? t("composer.model.maxOutput", { tokens: fmtTokens(tokenLimits.maxOutputTokens) })
      : null,
    model.outputModalities.length > 0
      ? t("composer.model.outputModalities", { modalities: model.outputModalities.join(" + ") })
      : null,
    model.toolUse ? t("composer.model.toolUse") : null,
    model.structuredOutput ? t("composer.model.structuredOutput") : null,
    model.knowledgeCutoff
      ? t("composer.model.knowledgeCutoff", { cutoff: model.knowledgeCutoff })
      : null,
  ]
    .filter((value): value is string => value !== null)
    .join(" · ");
}

function ModelPickerPlaceholder() {
  return <SkeletonControl />;
}

export function ModelPicker() {
  const t = useT();
  const { data: models = [], isLoading, isError } = useModels();
  const setModel = useSetComposerModelPreference();
  const selection = useSelectedModelSelection();
  const selected = selection?.model;
  const recent = useRecentModelsStore((state) => state.recent);
  const remember = useRecentModelsStore((state) => state.remember);
  const groups = useMemo(
    () => (selection ? modelGroups(models, selection, recent, t) : []),
    [models, recent, selection, t],
  );
  const modelsByItemId = useMemo(
    () => new Map(models.map((model) => [modelItemId(model), model])),
    [models],
  );

  if (models.length === 0) {
    if (isError) {
      return (
        <Button
          variant="wash"
          tone="negative"
          size="xs"
          disabled
          title={t("providers.models.error")}
        >
          <Icon name="alert" size="sm" />
          <span>{t("providers.models.error")}</span>
        </Button>
      );
    }
    if (!isLoading) return null;
    return <ModelPickerPlaceholder />;
  }

  if (!selected) return <ModelPickerPlaceholder />;

  return (
    <RailCatalogPicker
      groups={groups}
      openAtGroupId={
        groups.some((group) => group.id === selected.provider) ? selected.provider : groups[0]?.id
      }
      label={t("composer.switchModel")}
      heading={t("composer.model.title")}
      activeAccessory={
        !selection || !selection.model.reasoning ? undefined : selection.model.reasoningLevels
            .length > 0 ? (
          <EffortChip selection={selection} />
        ) : (
          <span
            title={t("composer.effort.automatic.title")}
            {...stylex.props(effortStyles.fixed, vocab.muted, typeStep.uiSm)}
          >
            {t("composer.effort.automatic")}
          </span>
        )
      }
      placeholder={t("composer.model.search.placeholder")}
      emptyLabel={t("composer.model.search.empty")}
      onSelect={(item) => {
        const model = modelsByItemId.get(item.id);
        if (!model) return;
        remember({ provider: model.provider, id: model.id });
        const reasoningEffort = model.reasoningLevelOrDefault(selection.reasoningEffort);
        setModel({
          kind: "explicit",
          provider: model.provider,
          model: model.id,
          ...(reasoningEffort ? { reasoningEffort } : {}),
        });
      }}
      trigger={
        <AgentComposerChip
          aria-label={t("composer.switchModel")}
          title={`${selected.label} · ${providerDisplayName(selected.provider)}`}
          shrink="gives"
          leading={<ProviderIcon provider={selected.provider} size="sm" />}
          label={effortSuffixed(selected.label, selection, t)}
        />
      }
      side="top"
      align="start"
    />
  );
}
