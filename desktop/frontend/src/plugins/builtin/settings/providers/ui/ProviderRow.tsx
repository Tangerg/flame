import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useRef, useState } from "react";
import { Badge, Button, Icon, ProviderIcon, TextField } from "@/ui";
import {
  type ProviderConfiguration,
  useProviderMutationMaterialGeneration,
  useUpdateProvider,
  useTestProvider,
} from "../application/providerConfig";
import { ProviderCredentialsDraft } from "../application/providerDraft";
import { useT } from "@/lib/i18n";
import { useAsyncFeedback } from "../../kit";
import { space, type as typeStep } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

const pr = stylex.create({
  head: {
    display: "grid",
    gridTemplateColumns: "24px minmax(0, 1fr) auto",
    alignItems: "center",
    gap: space.s3,
  },
  id: { textTransform: "capitalize" },
  // The key gets the wider share: a base URL is longer than a provider's name.
  fields: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 2fr) minmax(0, 3fr)",
    gap: space.s2,
  },
});

export function ProviderRow({ p }: { p: ProviderConfiguration }) {
  const t = useT();
  const update = useUpdateProvider();
  const test = useTestProvider();
  const [draft, setDraft] = useState(() => ProviderCredentialsDraft.initial(p));
  const [saving, setSaving] = useState(false);
  // `saving` lags a render, so it cannot be the re-entrancy guard: a second click before the
  // button re-renders disabled opens a second write that overwrites the first one's draft.
  const savingLatch = useRef(false);
  const materialGeneration = useProviderMutationMaterialGeneration();
  const { feedback, reset, fail, run } = useAsyncFeedback(materialGeneration);

  const enabled = p.configured;
  const fromEnv = p.credential?.fromEnvironment ?? false;
  const hasStoredKey = p.credential?.stored ?? false;
  const dirty = draft.dirty(p);
  const valid = draft.valid(p);

  const onSave = async () => {
    if (savingLatch.current) return;
    savingLatch.current = true;
    setSaving(true);
    reset();
    try {
      const saved = await update(draft.toUpdate(p));
      setDraft(ProviderCredentialsDraft.initial(saved));
    } catch (err) {
      if (wasGenerationRetired(err)) return;
      fail(err instanceof Error ? err.message : t("providers.error.save"));
    } finally {
      savingLatch.current = false;
      setSaving(false);
    }
  };

  const onClearKey = async () => {
    if (savingLatch.current) return;
    savingLatch.current = true;
    setSaving(true);
    reset();
    try {
      const saved = await update({ provider: p.id, apiKey: { type: "clear" } });
      setDraft(ProviderCredentialsDraft.initial(saved));
    } catch (err) {
      if (wasGenerationRetired(err)) return;
      fail(err instanceof Error ? err.message : t("providers.error.save"));
    } finally {
      savingLatch.current = false;
      setSaving(false);
    }
  };

  const onTest = () => run(() => test(p.id), t("providers.error.test"), wasGenerationRetired);

  return (
    <div {...stylex.props(ss.hoverRow, ss.hoverRowTall)}>
      <div {...stylex.props(pr.head)}>
        <ProviderIcon provider={p.id} size="lg" />
        <div {...stylex.props(ss.min)}>
          <div {...stylex.props(ss.truncate, ss.label, pr.id, typeStep.uiMd)}>{p.id}</div>
        </div>
        <Badge
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

      <div {...stylex.props(ss.afterRow, pr.fields)}>
        <TextField
          type="password"
          aria-label={t("providers.apiKey.aria", { provider: p.id })}
          value={draft.apiKey}
          onChange={(e) => setDraft((value) => value.withAPIKey(e.target.value))}
          placeholder={
            fromEnv
              ? t("providers.apiKey.envPlaceholder")
              : p.credential
                ? t("providers.apiKey.replace")
                : t("providers.apiKey.placeholder")
          }
        />
        <TextField
          type="text"
          aria-label={t("providers.baseUrl.aria", { provider: p.id })}
          value={draft.baseUrl}
          onChange={(e) => setDraft((value) => value.withBaseURL(e.target.value))}
          placeholder={t("providers.baseUrl.placeholder")}
        />
      </div>

      <div {...stylex.props(ss.afterRow, ss.line)}>
        <Button variant="primary" size="sm" disabled={!dirty || !valid || saving} onClick={onSave}>
          {saving ? t("providers.saving") : t("providers.save")}
        </Button>
        <Button
          variant="outline"
          size="sm"
          disabled={!enabled || feedback.state === "busy"}
          onClick={onTest}
        >
          {feedback.state === "busy" ? t("providers.testing") : t("providers.test")}
        </Button>
        {hasStoredKey && (
          <Button variant="ghost" size="sm" disabled={saving} onClick={onClearKey}>
            {t("providers.apiKey.clear")}
          </Button>
        )}

        {feedback.state === "ok" && (
          <span {...stylex.props(ss.inline, ss.success, typeStep.uiMd)}>
            <Icon name="check" size="sm" /> {t("providers.connectionOk")}
          </span>
        )}
        {feedback.state === "error" && (
          <span {...stylex.props(ss.inline, ss.min, ss.negative, typeStep.uiMd)}>
            <Icon name="alert" size="sm" />
            <span {...stylex.props(ss.truncate)} title={feedback.reason}>
              {feedback.reason}
            </span>
          </span>
        )}
      </div>
    </div>
  );
}
