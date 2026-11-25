import * as token from "@flame/ui/tokens.stylex";
import { useId, useState, useSyncExternalStore } from "react";
import type { ComponentContext, Translate } from "@flame/component-sdk";
import type { Provider } from "@flame/runtime-client/wire";
import {
  Badge,
  Button,
  FormActions,
  FormField,
  FormRow,
  ListRow,
  ModalDialog,
  TextField,
} from "@flame/ui";
import { ProviderCredentialsDraft } from "./draft";
import { providerResource, type ProviderSettings } from "./settings";
import { EndpointField } from "./endpointField";
import { CommandFeedback } from "./feedback";
const literal = { whiteSpace: "pre-wrap", overflowWrap: "anywhere" } as const;
const mono = { fontFamily: token.font.mono } as const;
const MASKED = "\u0000";

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
      <Badge tone="info" title={credential.masked}>
        {t("providers.fromEnv")}
      </Badge>
    );
  if (!provider.configured) return null;
  if (!credential) return <Badge tone="success">{t("providers.ready")}</Badge>;
  // Only the masked key is data; the surrounding words keep the body face.
  const [before = "", after = ""] = t("providers.key", {
    masked: MASKED,
  }).split(MASKED);
  return (
    <Badge tone="success">
      {before}
      <span style={mono}>{credential.masked}</span>
      {after}
    </Badge>
  );
}

// An environment key cannot be replaced here; a stored key can.
function apiKeyPlaceholder(provider: Provider, t: Translate) {
  if (provider.credential?.source === "env")
    return t("providers.apiKey.envPlaceholder");
  return provider.credential ? t("providers.apiKey.replace") : undefined;
}

// An unsaved edit is saved before it is tested.
function testLabel(dirty: boolean, probing: boolean) {
  if (dirty) return "providers.saveAndTest";
  return probing ? "providers.testing" : "providers.test";
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
  // An untouched, unconfigured row has nothing to save, test or report yet.
  const idle =
    !provider.configured && !dirty && !busy && feedback === undefined;
  const error = draft.endpointError(provider);
  const valid = error === undefined;
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
          gap: token.layout.contentGap,
          minWidth: 0,
        }}
      >
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
            placeholder={apiKeyPlaceholder(provider, t)}
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
      {!idle && (
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
            disabled={
              Boolean(command) || (dirty ? !valid : !provider.configured)
            }
            pending={probing}
            onClick={() => void settings.test(context, provider, draft)}
          >
            {t(testLabel(dirty, probing))}
          </Button>
          <Button
            variant="primary"
            size="sm"
            disabled={
              !dirty || !valid || probing || command?.phase === "unknown"
            }
            pending={command?.phase === "running"}
            onClick={() => void settings.save(context, provider, draft)}
          >
            {command?.phase === "running"
              ? t("providers.saving")
              : t("providers.save")}
          </Button>
        </FormActions>
      )}
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
