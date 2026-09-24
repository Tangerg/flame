import { useMemo } from "react";

import { fmtTokens } from "@/lib/format";
import { type Translate, useT } from "@/lib/i18n";
import {
  Button,
  Icon,
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
          label={selected.label}
        />
      }
      side="top"
      align="start"
    />
  );
}
