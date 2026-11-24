import { useId, useSyncExternalStore } from "react";
import {
  failureText,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import type { Model } from "@flame/runtime-client/wire";
import { Button, ListRow, Metadata, RecordBody, Select } from "@flame/ui";
import type { ProviderSettings, Read } from "./settings";
import { CommandFeedback } from "./feedback";
const column = {
  display: "grid",
  minWidth: 0,
  gap: "var(--layout-field-gap)",
} as const;
type Choice = { provider: string; model: string; label: string };
const choiceKey = (value: { provider?: string; model?: string }) =>
  JSON.stringify([value.provider ?? "", value.model ?? ""]);
function modelChoice(model: Model): Choice {
  return {
    provider: model.provider,
    model: model.id,
    label: model.displayName ?? model.id,
  };
}
const choiceLabel = (value: Choice) =>
  `${value.label} · ${value.provider}${value.label === value.model ? "" : ` / ${value.model}`}`;

type RoleKind = "utility" | "embedding";
type Snapshot = ReturnType<ProviderSettings["getSnapshot"]>;

const ROLES = {
  utility: {
    title: "providers.utility.title",
    description: "providers.utility.desc",
    unset: "providers.utility.main",
  },
  embedding: {
    title: "providers.embedding.title",
    description: "providers.embedding.desc",
    unset: "providers.embedding.off",
  },
} as const satisfies Record<RoleKind, Record<string, string>>;

// A utility role may use any listed model; an embedding role uses the preset
// embedding model of a configured provider that can embed.
function roleChoices(kind: RoleKind, state: Snapshot): Choice[] {
  if (kind === "utility") return (state.models.data ?? []).map(modelChoice);
  return (state.providers.data ?? [])
    .filter(
      (provider) =>
        provider.configured &&
        provider.embeddingCapable &&
        provider.defaultEmbeddingModel,
    )
    .map((provider) => ({
      provider: provider.id,
      model: provider.defaultEmbeddingModel!,
      label: provider.defaultEmbeddingModel!,
    }));
}

// A saved role that is no longer offered stays visible, disabled, under its
// saved name rather than silently reading as unset.
function roleOptions(
  choices: Choice[],
  unsetLabel: string,
  saved: { provider: string; model: string } | undefined,
) {
  const options: { value: string; label: string; disabled?: boolean }[] = [
    { value: "default", label: unsetLabel },
    ...choices.map((choice) => ({
      value: choiceKey(choice),
      label: choiceLabel(choice),
    })),
  ];
  if (
    saved &&
    !choices.some((choice) => choiceKey(choice) === choiceKey(saved))
  )
    options.push({
      value: choiceKey(saved),
      label: choiceLabel({ ...saved, label: saved.model }),
      disabled: true,
    });
  return options;
}

export function RoleSection({
  kind,
  context,
  settings,
  t,
}: {
  kind: RoleKind;
  context: ComponentContext;
  settings: ProviderSettings;
  t: Translate;
}) {
  const state = useSyncExternalStore(settings.subscribe, settings.getSnapshot);
  const read = state[kind];
  const role = read.data;
  const descriptionId = useId();
  const known = role !== undefined && state.providers.data !== undefined;
  const labels = ROLES[kind];
  const title = t(labels.title);
  const saved =
    role?.provider && role.model
      ? { provider: role.provider, model: role.model }
      : undefined;
  const configured =
    state.providers.data?.filter((provider) => provider.configured) ?? [];
  const embeddingProviders = configured.filter(
    (provider) => provider.embeddingCapable,
  );
  const choices = roleChoices(kind, state);
  const value = saved ? choiceKey(saved) : "default";
  const options = roleOptions(choices, t(labels.unset), saved);
  const command = state.commands.get(kind);
  const feedback = state.feedback.get(kind);
  const unknownFailure =
    !known && (read.phase === "failed" || state.providers.phase === "failed");
  const pick = (value: string) => {
    const choice = choices.find((choice) => choiceKey(choice) === value);
    if (value === "default" || choice)
      void settings.setRole(
        context,
        kind,
        choice ? { provider: choice.provider, model: choice.model } : {},
      );
  };
  const unavailable =
    saved !== undefined &&
    !configured.some((provider) => provider.id === saved.provider);
  return (
    <ListRow aria-busy={command?.phase === "running" || undefined}>
      <div
        data-focus-unit=""
        style={{
          display: "grid",
          gridTemplateColumns: "repeat(auto-fit,minmax(min(100%,17.5rem),1fr))",
          gap: "var(--layout-content-gap)",
          alignItems: "start",
          minWidth: 0,
        }}
      >
        <div style={column}>
          <strong>{title}</strong>
          <RecordBody>{t(labels.description)}</RecordBody>
          {(read.phase === "failed" || state.providers.phase === "failed") &&
            known && <Metadata>{t("providers.role.stale")}</Metadata>}
          {unavailable && <Metadata>{t("providers.notConfigured")}</Metadata>}
          {kind === "embedding" && known && choices.length === 0 && (
            <Metadata>
              {t(
                embeddingProviders.length
                  ? "providers.embedding.noPreset"
                  : "providers.embedding.none",
              )}
            </Metadata>
          )}
        </div>
        <div style={column}>
          {!known ? (
            unknownFailure ? (
              <>
                <p role="alert">{t("providers.role.loadError")}</p>
                <Button
                  aria-label={`${t("retry")} · ${title}`}
                  onClick={() => {
                    settings.refreshRead(context, kind);
                    settings.refreshRead(context, "providers");
                  }}
                >
                  {t("retry")}
                </Button>
              </>
            ) : (
              <Button variant="outline" disabled aria-label={title}>
                {t("reading")}
              </Button>
            )
          ) : (
            <>
              <span id={descriptionId} hidden>
                {options.find((option) => option.value === value)?.label}
              </span>
              <Select
                label={title}
                aria-describedby={descriptionId}
                value={value}
                options={options}
                disabled={Boolean(command)}
                focusableWhenDisabled
                onValueChange={pick}
              />
              {command?.phase === "running" && (
                <p role="status">{t("providers.saving")}</p>
              )}
            </>
          )}
          {kind === "utility" && (
            <ModelCatalogStatus
              read={state.models}
              t={t}
              refresh={() => settings.refreshRead(context, "models")}
            />
          )}
          <CommandFeedback
            context={context}
            settings={settings}
            resource={kind}
            command={command}
            feedback={feedback}
            t={t}
          />
        </div>
      </div>
    </ListRow>
  );
}

function ModelCatalogStatus({
  read,
  refresh,
  t,
}: {
  read: Read<Model[]>;
  refresh(): void;
  t: Translate;
}) {
  return (
    <div style={column}>
      {read.data === undefined && read.phase === "loading" && (
        <p role="status">{t("reading")}</p>
      )}
      {read.phase === "failed" && (
        <div role="alert">
          <RecordBody>
            {t("providers.models.error")} {failureText(read.error)}
          </RecordBody>
          {read.data !== undefined && <Metadata>{t("read.retained")}</Metadata>}
        </div>
      )}
      <Button
        variant="ghost"
        size="sm"
        pending={read.phase === "loading"}
        onClick={refresh}
      >
        {t("providers.models.refresh")}
      </Button>
    </div>
  );
}
