import { tupleKey } from "@/lib/tupleKey";
import { useMemo } from "react";
import { fmtTokens } from "@/lib/format";
import { type Translate, useT } from "@/lib/i18n/index";
import { Icon } from "@/ui/index";
import { ProviderIcon, providerDisplayName } from "@/ui/index";
import { RailCatalogPicker, type CatalogPickerGroup } from "@/ui/index";
import { SystemMessage } from "@/ui/index";
import { Button, FormActions } from "@flame/ui";
import { useWorkspace } from "@/features/workspace/public/workspace";
import { openWorkspaceSettingsPane } from "@/features/workspace/public/navigation";
import { useSettingsPaneForIntent } from "@/features/settings/public/intent";
import { type SelectableModel } from "@/features/providers/public/queries";
import { useModels } from "@/features/providers/public/queries";
import { useRecentModelsStore, type RecentModel } from "@/features/composer/adapters/recentModels";
import { AgentComposerChip } from "@/ui/agent/index";
import { useSetComposerModelPreference } from "@/features/composer/public/modelPreference";
import { useSelectedModelSelection } from "@/features/composer/public/selectedModel";
type Selection = NonNullable<ReturnType<typeof useSelectedModelSelection>>;

function modelItemId(model: SelectableModel): string {
  return tupleKey(model.provider, model.id);
}

const RECENT_GROUP_ID = "__recent";

function modelItem(model: SelectableModel, selection: Selection | undefined, t: Translate) {
  const active = model.provider === selection?.provider && model.id === selection.modelId;
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
  selection: Selection | undefined,
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

export function ModelPicker() {
  const providersPane = useSettingsPaneForIntent("modelProviders");
  const t = useT();
  const workspace = useWorkspace();
  const { data, isPending, isError, isFetching, refetch } = useModels();
  const setModel = useSetComposerModelPreference();
  const selection = useSelectedModelSelection();
  const selected = selection?.model;
  const recent = useRecentModelsStore((state) => state.recent);
  const remember = useRecentModelsStore((state) => state.remember);
  const groups = useMemo(
    () => modelGroups(data ?? [], selection, recent, t),
    [data, recent, selection, t],
  );
  const modelsByItemId = useMemo(
    () => new Map((data ?? []).map((model) => [modelItemId(model), model])),
    [data],
  );

  const label = selected?.label ?? selection?.modelId ?? t("composer.switchModel");

  return (
    <RailCatalogPicker
      groups={groups}
      openAtGroupId={
        selection && groups.some((group) => group.id === selection.provider)
          ? selection.provider
          : groups[0]?.id
      }
      label={t("composer.switchModel")}
      placeholder={t("composer.model.search.placeholder")}
      emptyLabel={
        data === undefined
          ? isPending
            ? t("common.loading")
            : null
          : t("composer.model.search.empty")
      }
      feedback={
        isError ? (
          <SystemMessage variant="warning">{t("composer.models.error")}</SystemMessage>
        ) : undefined
      }
      footer={
        <FormActions label={t("composer.switchModel")}>
          {data?.length === 0 && (
            <Button
              variant="ghost"
              size="sm"
              onClick={() => openWorkspaceSettingsPane(workspace, providersPane)}
            >
              {t("providers.setup.action")}
            </Button>
          )}
          <Button variant="ghost" size="sm" pending={isFetching} onClick={() => void refetch()}>
            <Icon name="loop" size="sm" />
            {t("composer.models.refresh")}
          </Button>
        </FormActions>
      }
      clearLabel={t("common.clearSearch")}
      onSelect={(item) => {
        const model = modelsByItemId.get(item.id);
        if (!model) return;
        remember({ provider: model.provider, id: model.id });
        const reasoningEffort = model.reasoningLevelOrDefault(selection?.reasoningEffort);
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
          title={
            selection
              ? `${label} · ${providerDisplayName(selection.provider)}${selected || data === undefined ? "" : ` · ${t("composer.model.unavailable")}`}`
              : undefined
          }
          shrink="gives"
          leading={
            selection ? (
              <ProviderIcon provider={selection.provider} size="composer" />
            ) : (
              <Icon name="server" size="composer" />
            )
          }
          label={label}
        />
      }
      side="top"
      align="start"
    />
  );
}
