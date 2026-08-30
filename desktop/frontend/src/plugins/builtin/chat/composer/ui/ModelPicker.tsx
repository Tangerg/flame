import { useMemo } from "react";

import { fmtTokens } from "@/lib/format";
import { useT } from "@/lib/i18n";
import {
  type SelectableModel,
  useModels,
} from "@/plugins/builtin/settings/providers/public/queries";
import { Button, CatalogPicker, type CatalogPickerGroup, Icon, ProviderIcon } from "@/ui";
import { AgentComposerChip } from "@/ui/agent";
import { useSetComposerModelPreference } from "../public/modelPreference";
import { useSelectedModelSelection } from "../public/selectedModel";

function modelItemId(model: SelectableModel): string {
  return JSON.stringify([model.provider, model.id]);
}

function groupModels(
  models: readonly SelectableModel[],
  selected: SelectableModel,
): CatalogPickerGroup[] {
  const byProvider = new Map<string, SelectableModel[]>();
  for (const model of models) {
    const items = byProvider.get(model.provider);
    if (items) items.push(model);
    else byProvider.set(model.provider, [model]);
  }

  const groups = [...byProvider].map(([provider, items]) => ({
    id: provider,
    label: provider.charAt(0).toUpperCase() + provider.slice(1),
    leading: <ProviderIcon provider={provider} size="sm" />,
    count: items.length,
    items: items.map((model) => ({
      id: modelItemId(model),
      label: model.label,
      leading: <ProviderIcon provider={model.provider} size="md" />,
      description: <ModelCapabilities model={model} />,
      keywords: [model.provider, model.id],
      active: model.provider === selected.provider && model.id === selected.id,
    })),
  }));
  return groups.sort((left, right) => {
    if (left.id === selected.provider) return -1;
    if (right.id === selected.provider) return 1;
    return 0;
  });
}

function ModelCapabilities({ model }: { model: SelectableModel }) {
  const t = useT();
  const tokenLimits = model.tokenLimits;
  const primary = [
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
  ].filter((value): value is string => value !== null);
  const secondary = [
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
  ].filter((value): value is string => value !== null);
  if (primary.length === 0 && secondary.length === 0) return null;

  const title = [...primary, ...secondary].join(" · ");
  return (
    <span
      className="block min-w-0 text-ui-xs font-normal text-fg-faint"
      title={title}
      aria-label={title}
    >
      {primary.length > 0 && <span className="block truncate">{primary.join(" · ")}</span>}
      {secondary.length > 0 && (
        <span className="block truncate opacity-80">{secondary.join(" · ")}</span>
      )}
    </span>
  );
}

function ModelPickerPlaceholder() {
  return (
    <div
      className="inline-flex h-[var(--control-height-md)] shrink-0 items-center gap-1.5 rounded-md px-2.5 opacity-60"
      aria-hidden
    >
      <span className="h-1.5 w-1.5 rounded-full bg-surface-2" />
      <span className="h-3 w-16 rounded-sm bg-surface-2" />
    </div>
  );
}

export function ModelPicker() {
  const t = useT();
  const { data: models = [], isLoading, isError } = useModels();
  const setModel = useSetComposerModelPreference();
  const selection = useSelectedModelSelection();
  const selected = selection?.model;
  const groups = useMemo(() => (selected ? groupModels(models, selected) : []), [models, selected]);
  const modelsByItemId = useMemo(
    () => new Map(models.map((model) => [modelItemId(model), model])),
    [models],
  );

  if (models.length === 0) {
    if (isError) {
      return (
        <Button
          variant="ghost"
          disabled
          title={t("providers.models.error")}
          className="gap-1.5 px-2 text-ui-sm text-negative"
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
    <CatalogPicker
      groups={groups}
      label={t("composer.switchModel")}
      placeholder={t("composer.model.search.placeholder")}
      emptyLabel={t("composer.model.search.empty")}
      onSelect={(item) => {
        const model = modelsByItemId.get(item.id);
        if (!model) return;
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
          shrink="gives"
          leading={<ProviderIcon provider={selected.provider} size="sm" />}
          label={selected.label}
        />
      }
      contentClassName="w-[380px]"
      side="top"
      align="start"
    />
  );
}
