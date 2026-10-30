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
import { CommandFeedback } from "./feedback";
const literal = { whiteSpace: "pre-wrap", overflowWrap: "anywhere" } as const;
export function ProviderRow({
  context,
  settings,
  provider: p,
  t,
}: {
  context: ComponentContext;
  settings: ProviderSettings;
  provider: Provider;
  t: Translate;
}) {
  const state = useSyncExternalStore(settings.subscribe, settings.getSnapshot);
  const draft = state.drafts.get(p.id) ?? ProviderCredentialsDraft.initial();
  const resource = providerResource(p.id);
  const command = state.commands.get(resource);
  const feedback = state.feedback.get(resource);
  const probing = state.probing.has(p.id);
  const busy = Boolean(command) || probing;
  const [confirmingClear, setConfirmingClear] = useState(false);
  const hintId = useId();
  const feedbackId = useId();
  const dirty = draft.dirty(p);
  const valid = draft.valid(p);
  const fromEnv = p.credential?.source === "env";
  const describedBy =
    feedback?.kind === "error" || command?.phase === "unknown"
      ? feedbackId
      : undefined;
  const change = (value: ProviderCredentialsDraft) =>
    settings.edit(context, p, value);
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
        <strong style={{ ...literal, flex: "1 1 10rem" }}>{p.id}</strong>
        <Badge
          tone={fromEnv ? "info" : p.configured ? "success" : "neutral"}
          title={fromEnv ? p.credential?.masked : undefined}
          face="mono"
        >
          {fromEnv
            ? t("providers.fromEnv")
            : p.configured
              ? p.credential
                ? t("providers.key", { masked: p.credential.masked })
                : t("providers.ready")
              : t("providers.notConfigured")}
        </Badge>
      </div>
      <FormRow>
        <FormField label={t("providers.apiKey.label")}>
          <TextField
            font="mono"
            type="password"
            autoComplete="off"
            aria-label={t("providers.apiKey.aria", { provider: p.id })}
            aria-describedby={describedBy}
            value={draft.apiKey}
            onChange={(event) => change(draft.withAPIKey(event.target.value))}
            placeholder={t(
              fromEnv
                ? "providers.apiKey.envPlaceholder"
                : p.credential
                  ? "providers.apiKey.replace"
                  : "providers.apiKey.placeholder",
            )}
          />
        </FormField>
        <FormField
          label={t(
            p.requiresBaseUrl
              ? "providers.baseUrl.labelRequired"
              : "providers.baseUrl.label",
          )}
        >
          <TextField
            font="mono"
            type="text"
            required={p.requiresBaseUrl}
            invalid={dirty && !valid}
            aria-label={t("providers.baseUrl.aria", { provider: p.id })}
            aria-describedby={
              [describedBy, dirty && !valid ? hintId : undefined]
                .filter(Boolean)
                .join(" ") || undefined
            }
            value={draft.baseURL(p)}
            onChange={(event) => change(draft.withBaseURL(event.target.value))}
            placeholder={t("providers.baseUrl.placeholder")}
          />
        </FormField>
      </FormRow>
      <FormActions
        label={t("providers.save")}
        auxiliaryAction={
          p.credential?.source === "stored" && (
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
          disabled={Boolean(command) || (dirty ? !valid : !p.configured)}
          pending={probing}
          onClick={() => void settings.test(context, p, draft)}
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
          pending={command?.phase === "running"}
          onClick={() => void settings.save(context, p, draft)}
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
      {dirty && !valid && <p id={hintId}>{t("providers.baseUrl.required")}</p>}
      <ModalDialog
        open={confirmingClear}
        onOpenChange={setConfirmingClear}
        title={t("providers.apiKey.clear.title")}
        description={t("providers.apiKey.clear.body", { provider: p.id })}
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
                void settings.clearKey(context, p.id);
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
