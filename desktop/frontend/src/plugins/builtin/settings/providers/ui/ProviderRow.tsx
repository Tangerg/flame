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
    <div className="rounded-md px-3 py-3 transition-colors hover:bg-hover">
      <div className="grid grid-cols-[24px_minmax(0,1fr)_auto] items-center gap-3">
        <ProviderIcon provider={p.id} size="lg" />
        <div className="min-w-0">
          <div className="truncate text-ui-md font-medium capitalize text-fg">{p.id}</div>
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

      <div className="mt-2.5 grid grid-cols-[minmax(0,2fr)_minmax(0,3fr)] gap-2">
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

      <div className="mt-2.5 flex items-center gap-2">
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
          <span className="inline-flex items-center gap-1 text-ui-md text-success">
            <Icon name="check" size="sm" /> {t("providers.connectionOk")}
          </span>
        )}
        {feedback.state === "error" && (
          <span className="inline-flex min-w-0 items-center gap-1 text-ui-md text-negative">
            <Icon name="alert" size="sm" />
            <span className="truncate" title={feedback.reason}>
              {feedback.reason}
            </span>
          </span>
        )}
      </div>
    </div>
  );
}
