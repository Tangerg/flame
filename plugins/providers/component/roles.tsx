import * as token from "@flame/ui/tokens.stylex";
import { useId, useSyncExternalStore, type CSSProperties } from "react";
import {
  failureText,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import type { Model } from "@flame/runtime-client/wire";
import {
  Button,
  FormRow,
  ListRow,
  Metadata,
  RecordBody,
  RecordHeading,
  Select,
} from "@flame/ui";
import type { ProviderSettings, Read, RoleKind } from "./settings";
import { CommandFeedback } from "./feedback";
const column = {
  display: "grid",
  minWidth: 0,
  gap: token.layout.fieldGap,
} as const;
// The control sits in a FormRow's second column like the provider forms above it: the
// Select's fill hangs by the row bleed so its text starts on that column's label line.
const control = {
  ...column,
  "--field-hang": token.space.s3,
} as CSSProperties;
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

// The unset choice: the Runtime falls back to its configured default model.
const DEFAULT_ROLE = "default";

// A saved role that is no longer offered stays visible, disabled, under its
// saved name rather than silently reading as unset.
function roleOptions(
  choices: Choice[],
  unsetLabel: string,
  saved: { provider: string; model: string } | undefined,
) {
  const options: { value: string; label: string; disabled?: boolean }[] = [
    { value: DEFAULT_ROLE, label: unsetLabel },
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

function RoleNotes({
  t,
  kind,
  stale,
  unavailable,
  choiceCount,
  embeddingProviderCount,
}: {
  t: Translate;
  kind: RoleKind;
  stale: boolean;
  unavailable: boolean;
  choiceCount: number;
  embeddingProviderCount: number;
}) {
  return (
    <>
      {stale && <Metadata>{t("providers.role.stale")}</Metadata>}
      {unavailable && <Metadata>{t("providers.notConfigured")}</Metadata>}
      {kind === "embedding" && choiceCount === 0 && (
        <Metadata>
          {t(
            embeddingProviderCount
              ? "providers.embedding.noPreset"
              : "providers.embedding.none",
          )}
        </Metadata>
      )}
    </>
  );
}

function RolePending({
  t,
  title,
  failed,
  onRetry,
}: {
  t: Translate;
  title: string;
  failed: boolean;
  onRetry(): void;
}) {
  if (!failed)
    return (
      <Button variant="soft" disabled aria-label={title}>
        {t("reading")}
      </Button>
    );
  return (
    <>
      <p role="alert">{t("providers.role.loadError")}</p>
      <Button aria-label={`${t("retry")} · ${title}`} onClick={onRetry}>
        {t("retry")}
      </Button>
    </>
  );
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
  const value = saved ? choiceKey(saved) : DEFAULT_ROLE;
  const options = roleOptions(choices, t(labels.unset), saved);
  const command = state.commands.get(kind);
  const feedback = state.feedback.get(kind);
  const failed = read.phase === "failed" || state.providers.phase === "failed";
  const pick = (value: string) => {
    const choice = choices.find((choice) => choiceKey(choice) === value);
    if (value === DEFAULT_ROLE || choice)
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
      <div data-focus-unit="">
        <FormRow>
          <div style={column}>
            <RecordHeading title={title} description={t(labels.description)} />
            {known && (
              <RoleNotes
                t={t}
                kind={kind}
                stale={failed}
                unavailable={unavailable}
                choiceCount={choices.length}
                embeddingProviderCount={embeddingProviders.length}
              />
            )}
          </div>
          <div style={control}>
            {!known ? (
              <RolePending
                t={t}
                title={title}
                failed={failed}
                onRetry={() => {
                  settings.refreshRead(context, kind);
                  settings.refreshRead(context, "providers");
                }}
              />
            ) : (
              <>
                <Select
                  label={title}
                  aria-describedby={descriptionId}
                  value={value}
                  options={options}
                  disabled={Boolean(command)}
                  focusableWhenDisabled
                  onValueChange={pick}
                />
                <span id={descriptionId} hidden>
                  {options.find((option) => option.value === value)?.label}
                </span>
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
        </FormRow>
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
  if (read.data === undefined && read.phase === "loading")
    return <p role="status">{t("reading")}</p>;
  if (read.phase !== "failed") return null;
  return (
    <div role="alert">
      <RecordBody>
        {t("providers.models.error")} {failureText(read.error)}
      </RecordBody>
      {read.data !== undefined && <Metadata>{t("read.retained")}</Metadata>}
      <Button onClick={refresh}>{t("retry")}</Button>
    </div>
  );
}
