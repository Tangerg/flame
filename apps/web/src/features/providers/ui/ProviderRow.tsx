import { failureText } from "@/lib/rpcErrors";
import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useId, useRef, useState } from "react";
import { ConfirmDialog } from "@/ui/index";
import { Icon } from "@/ui/index";
import { SystemMessage } from "@/ui/index";
import { ProviderIcon } from "@/ui/index";
import {
  Badge,
  Button,
  FormActions,
  FormField,
  FormRow,
  ListRow,
  TextField,
  vocab,
} from "@flame/ui";
import { type ProviderConfiguration } from "@/features/providers/application/providerConfig";
import {
  useProviderMutationMaterialGeneration,
  useUpdateProvider,
  useTestProvider,
} from "@/features/providers/application/providerConfig";
import { type ProviderCredentialsDraft } from "@/features/providers/application/providerDraft";
import { useProviderDraft } from "@/features/providers/application/providerDrafts";
import { useT } from "@/lib/i18n/index";
import { useAsyncFeedback } from "@/lib/useAsyncFeedback";
import { layout, type as typeStep } from "@flame/ui/tokens.stylex";
import { settingStyles as ss } from "@/ui/settings/settingStyles";
const pr = stylex.create({
  head: {
    display: "grid",
    gridTemplateColumns: {
      default: "24px minmax(0, 1fr) auto",
      "@container (max-width: 440px)": "24px minmax(0,1fr)",
    },
    alignItems: "center",
    gap: layout.contentGap,
  },
  id: { textTransform: "capitalize" },
  status: {
    gridColumn: { default: null, "@container (max-width: 440px)": "2 / -1" },
    justifySelf: "start",
  },
  statusMessage: { ":empty": { display: "none" } },
});

export function ProviderRow({ p }: { p: ProviderConfiguration }) {
  const t = useT();
  const update = useUpdateProvider();
  const test = useTestProvider();
  const [draft, setDraft] = useProviderDraft(p);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [confirmingClear, setConfirmingClear] = useState(false);
  const savingLatch = useRef(false);
  const materialGeneration = useProviderMutationMaterialGeneration();
  const { feedback, reset, fail, run } = useAsyncFeedback(materialGeneration);
  const feedbackId = useId();

  const enabled = p.configured;
  const fromEnv = p.credential?.fromEnvironment ?? false;
  const hasStoredKey = p.credential?.stored ?? false;
  const dirty = draft.dirty(p);
  const valid = draft.valid(p);

  const edit = (change: (value: ProviderCredentialsDraft) => ProviderCredentialsDraft) => {
    setDraft(change);
    setSaved(false);
    reset();
  };

  const saveFailure = (err: unknown) => failureText(err, t("providers.error.save"));

  const commit = async (op: () => Promise<ProviderConfiguration>) => {
    savingLatch.current = true;
    setSaving(true);
    setSaved(false);
    try {
      return await op();
    } finally {
      savingLatch.current = false;
      setSaving(false);
    }
  };

  const saveDraft = async () => {
    const submitted = draft;
    const result = await commit(() => update(submitted.toUpdate(p)));
    setDraft((current) => current.settle(submitted, result));
  };

  const mutate = async (op: () => Promise<unknown>) => {
    if (savingLatch.current) return false;
    reset();
    try {
      await op();
      return true;
    } catch (err) {
      if (!wasGenerationRetired(err)) fail(saveFailure(err));
      return false;
    }
  };

  const onSave = async () => {
    if (await mutate(saveDraft)) setSaved(true);
  };

  const onClearKey = () =>
    mutate(() => commit(() => update({ provider: p.id, apiKey: { type: "clear" } })));

  const onTest = () => {
    if (savingLatch.current) return;
    setSaved(false);
    void run(
      async () => {
        if (dirty) {
          try {
            await saveDraft();
          } catch (err) {
            if (wasGenerationRetired(err)) throw err;
            return { ok: false, error: saveFailure(err) };
          }
        }
        return test(p.id);
      },
      t("providers.error.test"),
      wasGenerationRetired,
    );
  };

  const describedBy = feedback.state === "error" ? feedbackId : undefined;

  return (
    <ListRow>
      <div {...stylex.props(pr.head)}>
        <ProviderIcon provider={p.id} size="lg" />
        <div {...stylex.props(vocab.min)}>
          <div {...stylex.props(vocab.truncate, ss.label, pr.id, typeStep.uiMd)}>{p.id}</div>
        </div>
        <Badge
          styles={pr.status}
          size="md"
          tone={fromEnv ? "info" : enabled ? "success" : "neutral"}
          title={fromEnv ? p.credential?.masked : undefined}
          face="mono"
        >
          {fromEnv
            ? t("providers.fromEnv")
            : enabled
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
            onChange={(e) => edit((value) => value.withAPIKey(e.target.value))}
            placeholder={
              fromEnv
                ? t("providers.apiKey.envPlaceholder")
                : p.credential
                  ? t("providers.apiKey.replace")
                  : t("providers.apiKey.placeholder")
            }
          />
        </FormField>
        <FormField
          label={t(
            p.requiresBaseUrl ? "providers.baseUrl.labelRequired" : "providers.baseUrl.label",
          )}
        >
          <TextField
            font="mono"
            type="text"
            required={p.requiresBaseUrl}
            invalid={dirty && !valid}
            aria-label={t("providers.baseUrl.aria", { provider: p.id })}
            aria-describedby={describedBy}
            value={draft.baseUrl}
            onChange={(e) => edit((value) => value.withBaseURL(e.target.value))}
            placeholder={t("providers.baseUrl.placeholder")}
          />
        </FormField>
      </FormRow>

      <FormActions
        label={t("providers.save")}
        auxiliaryAction={
          hasStoredKey && (
            <Button
              variant="ghost"
              size="sm"
              pending={saving}
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
          disabled={dirty ? !valid : !enabled}
          pending={feedback.state === "busy" || saving}
          onClick={onTest}
        >
          {dirty
            ? t("providers.saveAndTest")
            : feedback.state === "busy"
              ? t("providers.testing")
              : t("providers.test")}
        </Button>
        <Button
          variant="primary"
          size="sm"
          disabled={!dirty || !valid}
          pending={saving}
          onClick={() => void onSave()}
        >
          {saving ? t("providers.saving") : t("providers.save")}
        </Button>
      </FormActions>

      <span
        role="status"
        aria-live="polite"
        {...stylex.props(pr.statusMessage, ss.inline, typeStep.uiMd)}
      >
        {feedback.state === "ok" ? (
          <span {...stylex.props(ss.inline, vocab.success)}>
            <Icon name="check" size="sm" /> {t("providers.connectionOk")}
          </span>
        ) : saved ? (
          <span {...stylex.props(ss.inline, vocab.success)}>
            <Icon name="check" size="sm" /> {t("providers.saved")}
          </span>
        ) : null}
      </span>
      {feedback.state === "error" && (
        <SystemMessage id={feedbackId} variant="warning">
          {feedback.reason}
        </SystemMessage>
      )}
      {dirty && !valid && (
        <p {...stylex.props(ss.hintSpaced, typeStep.uiSm)}>{t("providers.baseUrl.required")}</p>
      )}
      <ConfirmDialog
        open={confirmingClear}
        onOpenChange={setConfirmingClear}
        title={t("providers.apiKey.clear.title")}
        body={t("providers.apiKey.clear.body", { provider: p.id })}
        confirmLabel={t("providers.apiKey.clear")}
        cancelLabel={t("common.cancel")}
        destructive
        onConfirm={() => void onClearKey()}
      />
    </ListRow>
  );
}
