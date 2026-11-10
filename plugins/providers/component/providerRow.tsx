import { useId, useState, useSyncExternalStore } from "react";
import type { ComponentContext, Translate } from "@flame/component-sdk";
import type { Provider } from "@flame/runtime-client/wire";
import { Server } from "@flame/icons/react";
import {
  Badge,
  Button,
  FormActions,
  FormField,
  FormRow,
  Icon,
  ListRow,
  ModalDialog,
  TextField,
} from "@flame/ui";
import { ProviderCredentialsDraft } from "./draft";
import { providerResource, type ProviderSettings } from "./settings";
import { EndpointField } from "./endpointField";
import { CommandFeedback } from "./feedback";
const literal = { whiteSpace: "pre-wrap", overflowWrap: "anywhere" } as const;

function ProviderCredentialBadge({
  provider,
  t,
}: {
  provider: Provider;
  t: Translate;
}) {
  const credential = provider.credential;
  if (credential?.source === "env")
    return (
      <Badge tone="info" title={credential.masked} face="mono">
        {t("providers.fromEnv")}
      </Badge>
    );
  if (!provider.configured)
    return (
      <Badge tone="neutral" face="mono">
        {t("providers.notConfigured")}
      </Badge>
    );
  return (
    <Badge tone="success" face="mono">
      {credential
        ? t("providers.key", { masked: credential.masked })
        : t("providers.ready")}
    </Badge>
  );
}

export function ProviderRow({
  context,
  settings,
  provider,
  t,
}: {
  context: ComponentContext;
  settings: ProviderSettings;
  provider: Provider;
  t: Translate;
}) {
  const state = useSyncExternalStore(settings.subscribe, settings.getSnapshot);
  const draft =
    state.drafts.get(provider.id) ?? ProviderCredentialsDraft.initial();
  const resource = providerResource(provider.id);
  const command = state.commands.get(resource);
  const feedback = state.feedback.get(resource);
  const probing = state.probing.has(provider.id);
  const busy = Boolean(command) || probing;
  const [confirmingClear, setConfirmingClear] = useState(false);
  const feedbackId = useId();
  const dirty = draft.dirty(provider);
  const error = draft.endpointError(provider);
  const valid = error === undefined;
  const fromEnv = provider.credential?.source === "env";
  const describedBy =
    feedback?.kind === "error" || command?.phase === "unknown"
      ? feedbackId
      : undefined;
  const change = (value: ProviderCredentialsDraft) =>
    settings.edit(context, provider, value);
  return (
    <ListRow>
      <div
        style={{
          display: "flex",
          flexWrap: "wrap",
          alignItems: "center",
          gap: "var(--layout-content-gap)",
          minWidth: 0,
        }}
      >
        <Icon glyph={Server} size="lg" />
        <strong style={{ ...literal, flex: "1 1 10rem" }}>{provider.id}</strong>
        <ProviderCredentialBadge provider={provider} t={t} />
      </div>
      <FormRow>
        <FormField label={t("providers.apiKey.label")}>
          <TextField
            font="mono"
            type="password"
            autoComplete="off"
            aria-label={t("providers.apiKey.aria", { provider: provider.id })}
            aria-describedby={describedBy}
            value={draft.apiKey}
            onChange={(event) => change(draft.withAPIKey(event.target.value))}
            placeholder={t(
              fromEnv
                ? "providers.apiKey.envPlaceholder"
                : provider.credential
                  ? "providers.apiKey.replace"
                  : "providers.apiKey.placeholder",
            )}
          />
        </FormField>
        <EndpointField
          provider={provider}
          draft={draft}
          error={dirty ? error : undefined}
          feedback={describedBy}
          change={change}
          t={t}
        />
      </FormRow>
      <FormActions
        label={t("providers.save")}
        auxiliaryAction={
          provider.credential?.source === "stored" && (
            <Button
              variant="ghost"
              size="sm"
              disabled={busy}
              onClick={() => setConfirmingClear(true)}
            >
              {t("providers.apiKey.clear")}
            </Button>
          )
        }
      >
        <Button
          variant="outline"
          size="sm"
          disabled={Boolean(command) || (dirty ? !valid : !provider.configured)}
          pending={probing}
          onClick={() => void settings.test(context, provider, draft)}
        >
          {dirty
            ? t("providers.saveAndTest")
            : probing
              ? t("providers.testing")
              : t("providers.test")}
        </Button>
        <Button
          variant="primary"
          size="sm"
          disabled={!dirty || !valid || probing || command?.phase === "unknown"}
          focusableWhenDisabled={
            feedback?.kind === "saved" || command?.phase === "unknown"
          }
          pending={command?.phase === "running"}
          onClick={() => void settings.save(context, provider, draft)}
        >
          {command?.phase === "running"
            ? t("providers.saving")
            : t("providers.save")}
        </Button>
      </FormActions>
      <div id={feedbackId}>
        <CommandFeedback
          context={context}
          settings={settings}
          resource={resource}
          command={command}
          feedback={feedback}
          t={t}
        />
      </div>
      <ModalDialog
        open={confirmingClear}
        onOpenChange={setConfirmingClear}
        title={t("providers.apiKey.clear.title")}
        description={t("providers.apiKey.clear.body", {
          provider: provider.id,
        })}
        role="alertdialog"
        closeLabel={t("close")}
        footer={
          <>
            <Button onClick={() => setConfirmingClear(false)}>
              {t("cancel")}
            </Button>
            <Button
              tone="negative"
              disabled={busy}
              onClick={() => {
                setConfirmingClear(false);
                void settings.clearKey(context, provider.id);
              }}
            >
              {t("providers.apiKey.clear")}
            </Button>
          </>
        }
      />
    </ListRow>
  );
}
