import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useId, type ReactNode } from "react";
import { DropdownMenu, Icon, ProviderIcon, SelectTrigger } from "@/ui";
import { Button, gap, vocab } from "@flame/ui";
import {
  type RoleConfigState,
  setEmbeddingRole,
  setUtilityRole,
  useEmbeddingModelConfig,
  useProviderMutationMaterialGeneration,
  useUtilityModelConfig,
} from "../application/providerConfig";
import type { ProviderRole } from "../application/providerModels";
import { tupleKey } from "@/lib/tupleKey";
import { useT } from "@/lib/i18n";
import { SettingRow, useAsyncFeedback } from "../../settings/kit";
import { face, type as typeStep } from "@flame/ui/tokens.stylex";
import { settingStyles as ss } from "../../settings/kit/settingStyles";

const rs = stylex.create({
  trigger: {
    width: "fit-content",
    maxWidth: "20em",
    height: "auto",
    justifyContent: "space-between",
    textAlign: "start",
  },
  model: {
    display: "flex",
    flexDirection: "column",
    minWidth: 0,
    whiteSpace: "normal",
    overflowWrap: "anywhere",
  },
});

interface ModelChoice {
  provider: string;
  model: string;
  label: string;
}

function ModelLabel({ provider, model, label }: ModelChoice) {
  return (
    <span {...stylex.props(rs.model, typeStep.uiSm)}>
      <span>{label}</span>{" "}
      <span {...stylex.props(face.mono, typeStep.uiXs, vocab.muted)}>
        {provider}
        {label !== model && ` / ${model}`}
      </span>
    </span>
  );
}

function RoleModelPicker({
  title,
  defaultLabel,
  role,
  isSet,
  choices,
  busy,
  onPick,
}: {
  title: string;
  defaultLabel: string;
  role?: ProviderRole;
  isSet: boolean;
  choices: readonly ModelChoice[];
  busy: boolean;
  onPick: (role: ProviderRole | null) => Promise<void>;
}) {
  const t = useT();
  const valueId = useId();
  const selected = choices.find(
    (choice) => choice.provider === role?.provider && choice.model === role?.model,
  );
  return (
    <DropdownMenu.Root>
      <DropdownMenu.Trigger
        render={
          <SelectTrigger
            pending={busy}
            aria-label={title}
            aria-describedby={valueId}
            styles={rs.trigger}
            leading={
              busy ? (
                <Icon name="loop" size="xs" {...stylex.props(ss.spin, vocab.muted)} />
              ) : isSet && role?.provider ? (
                <ProviderIcon provider={role.provider} size="sm" />
              ) : undefined
            }
            label={
              <span id={valueId}>
                {busy ? (
                  <span {...stylex.props(vocab.muted)}>{t("providers.saving")}</span>
                ) : isSet && role?.provider ? (
                  <ModelLabel
                    provider={role.provider}
                    model={role.model ?? ""}
                    label={selected?.label ?? role.model ?? ""}
                  />
                ) : (
                  <span {...stylex.props(vocab.muted)}>{defaultLabel}</span>
                )}
              </span>
            }
          />
        }
      />
      <DropdownMenu.Content align="end" sideOffset={6}>
        <DropdownMenu.RadioGroup
          value={isSet ? (selected ?? role) : null}
          disabled={busy}
          onValueChange={(choice: ModelChoice | null) =>
            void onPick(choice ? { provider: choice.provider, model: choice.model } : null)
          }
        >
          <DropdownMenu.RadioItem value={null} layout="pick" label={defaultLabel}>
            <span />
            <span {...stylex.props(rs.model)}>{defaultLabel}</span>
            <DropdownMenu.RadioItemIndicator>
              <Icon name="check" size="xs" {...stylex.props(vocab.accent)} />
            </DropdownMenu.RadioItemIndicator>
          </DropdownMenu.RadioItem>
          {choices.map((choice) => (
            <DropdownMenu.RadioItem
              key={tupleKey(choice.provider, choice.model)}
              value={choice}
              layout="pick"
              label={choice.label}
            >
              <ProviderIcon provider={choice.provider} size="md" />
              <ModelLabel {...choice} />
              <DropdownMenu.RadioItemIndicator>
                <Icon name="check" size="xs" {...stylex.props(vocab.accent)} />
              </DropdownMenu.RadioItemIndicator>
            </DropdownMenu.RadioItem>
          ))}
        </DropdownMenu.RadioGroup>
      </DropdownMenu.Content>
    </DropdownMenu.Root>
  );
}

function RoleSectionShell({
  title,
  description,
  state,
  error,
  note,
  children,
}: {
  title: string;
  description: string;
  state: RoleConfigState;
  error?: string | null;
  note?: ReactNode;
  children: ReactNode;
}) {
  const t = useT();
  return (
    <SettingRow
      label={title}
      sub={
        <div {...stylex.props(vocab.column, gap.s1)}>
          <p>{description}</p>
          {state.kind === "ready" && note}
          {state.kind === "error" && (
            <p role="alert" {...stylex.props(vocab.negative)}>
              {t("providers.role.loadError")}
            </p>
          )}
          {state.kind === "ready" && state.stale && <p>{t("providers.role.stale")}</p>}
          {error && (
            <p role="alert" {...stylex.props(vocab.negative)}>
              {error}
            </p>
          )}
        </div>
      }
    >
      {state.kind === "loading" ? (
        <Button type="button" variant="outline" size="md" press="none" disabled aria-label={title}>
          <span {...stylex.props(vocab.muted)}>{t("common.loading")}</span>
        </Button>
      ) : state.kind === "error" ? (
        <Button type="button" variant="outline" size="md" onClick={state.retry}>
          {t("common.retry")}
        </Button>
      ) : (
        children
      )}
    </SettingRow>
  );
}

export function UtilityModelSection() {
  const t = useT();
  const { role, modelOptions, isSet, isAvailable, isError, state } = useUtilityModelConfig();
  const materialGeneration = useProviderMutationMaterialGeneration();
  const { feedback, run } = useAsyncFeedback(materialGeneration);
  const pick = (next: ProviderRole | null): Promise<void> =>
    run(() => setUtilityRole(next ?? {}), t("providers.utility.error"), wasGenerationRetired);

  return (
    <RoleSectionShell
      title={t("providers.utility.title")}
      description={t("providers.utility.desc")}
      state={state}
      error={
        feedback.state === "error" ? feedback.reason : isError ? t("providers.models.error") : null
      }
      note={isSet && !isAvailable ? <p>{t("providers.notConfigured")}</p> : null}
    >
      <RoleModelPicker
        title={t("providers.utility.title")}
        defaultLabel={t("providers.utility.main")}
        role={role}
        isSet={isSet}
        choices={modelOptions.map((model) => ({
          provider: model.provider,
          model: model.id,
          label: model.label,
        }))}
        busy={feedback.state === "busy"}
        onPick={pick}
      />
    </RoleSectionShell>
  );
}

export function EmbeddingModelSection() {
  const t = useT();
  const { role, capableProviders, isSet, isAvailable, state } = useEmbeddingModelConfig();
  const materialGeneration = useProviderMutationMaterialGeneration();
  const { feedback, run } = useAsyncFeedback(materialGeneration);
  const pick = (next: ProviderRole | null): Promise<void> =>
    run(() => setEmbeddingRole(next ?? {}), t("providers.embedding.error"), wasGenerationRetired);

  return (
    <RoleSectionShell
      title={t("providers.embedding.title")}
      description={t("providers.embedding.desc")}
      state={state}
      error={feedback.state === "error" ? feedback.reason : null}
      note={
        isSet && !isAvailable ? (
          <p>{t("providers.notConfigured")}</p>
        ) : capableProviders.length === 0 ? (
          <p>{t("providers.embedding.none")}</p>
        ) : null
      }
    >
      <RoleModelPicker
        title={t("providers.embedding.title")}
        defaultLabel={t("providers.embedding.off")}
        role={role}
        isSet={isSet}
        choices={capableProviders.map((provider) => ({
          provider: provider.id,
          model: provider.defaultEmbeddingModel || "",
          label: provider.defaultEmbeddingModel || provider.id,
        }))}
        busy={feedback.state === "busy"}
        onPick={pick}
      />
    </RoleSectionShell>
  );
}
