import { failureText } from "@/lib/rpcErrors";
import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useId, useRef, useState } from "react";
import { Icon, ProviderIcon } from "@/ui";
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
import {
  type ProviderConfiguration,
  useProviderMutationMaterialGeneration,
  useUpdateProvider,
  useTestProvider,
} from "../application/providerConfig";
import type { ProviderCredentialsDraft } from "../application/providerDraft";
import { useProviderDraft } from "../application/providerDrafts";
import { useT } from "@/lib/i18n";
import { useAsyncFeedback } from "../../settings/kit";
import { space, type as typeStep } from "@flame/ui/tokens.stylex";
import { settingStyles as ss } from "../../settings/kit/settingStyles";

const pr = stylex.create({
  head: {
    display: "grid",
    gridTemplateColumns: {
      default: "24px minmax(0, 1fr) auto",
      "@container (max-width: 440px)": "24px minmax(0,1fr)",
    },
    alignItems: "center",
    gap: space.s3,
  },
  id: { textTransform: "capitalize" },
  status: {
    gridColumn: { default: null, "@container (max-width: 440px)": "2 / -1" },
    justifySelf: "start",
  },
  statusMessage: { ":empty": { display: "none" } },
  reason: { minWidth: 0, overflowWrap: "anywhere", userSelect: "text" },
});

export function ProviderRow({ p }: { p: ProviderConfiguration }) {
  const t = useT();
  const update = useUpdateProvider();
  const test = useTestProvider();
  const [draft, setDraft] = useProviderDraft(p);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
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

  const mutate = async (op: () => Promise<ProviderConfiguration>) => {
    if (savingLatch.current) return null;
    savingLatch.current = true;
    setSaving(true);
    setSaved(false);
    reset();
    try {
      return await op();
    } catch (err) {
      if (!wasGenerationRetired(err)) {
        fail(failureText(err, t("providers.error.save")));
      }
      return null;
    } finally {
      savingLatch.current = false;
      setSaving(false);
    }
  };

  const save = async () => {
    const submitted = draft;
    const result = await mutate(() => update(submitted.toUpdate(p)));
    if (!result) return false;
    setDraft((current) => current.settle(submitted, result));
    setSaved(true);
    return true;
  };

  const onClearKey = () => mutate(() => update({ provider: p.id, apiKey: { type: "clear" } }));

  const onTest = async () => {
    if (dirty && !(await save())) return;
    setSaved(false);
    void run(() => test(p.id), t("providers.error.test"), wasGenerationRetired);
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

      <FormActions label={t("providers.save")}>
        <Button
          variant="outline"
          size="sm"
          disabled={dirty ? !valid : !enabled}
          pending={feedback.state === "busy" || saving}
          onClick={() => void onTest()}
        >
          {feedback.state === "busy"
            ? t("providers.testing")
            : dirty
              ? t("providers.saveAndTest")
              : t("providers.test")}
        </Button>
        {hasStoredKey && (
          <Button variant="ghost" size="sm" pending={saving} onClick={() => void onClearKey()}>
            {t("providers.apiKey.clear")}
          </Button>
        )}

        <Button
          variant="primary"
          size="sm"
          disabled={!dirty || !valid}
          pending={saving}
          onClick={() => void save()}
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
        <p
          id={feedbackId}
          role="alert"
          {...stylex.props(ss.inline, vocab.negative, pr.reason, typeStep.uiMd)}
        >
          <Icon name="alert" size="sm" />
          {feedback.reason}
        </p>
      )}
      {dirty && !valid && (
        <p {...stylex.props(ss.hintSpaced, typeStep.uiSm)}>{t("providers.baseUrl.required")}</p>
      )}
    </ListRow>
  );
}
