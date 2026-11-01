import { useId } from "react";
import type { Translate } from "@flame/component-sdk";
import type { Provider } from "@flame/runtime-client/wire";
import { FormField, TextField } from "@flame/ui";
import type { ProviderCredentialsDraft } from "./draft";

function pathHint(
  endpoint: Provider["endpoint"],
  path: string | undefined,
  t: Translate,
) {
  if (!endpoint) return undefined;
  if (endpoint.appendsPath) {
    return path
      ? t("providers.baseUrl.appends", { suffix: endpoint.appendsPath, path })
      : t("providers.baseUrl.mount", { suffix: endpoint.appendsPath });
  }
  if (endpoint.requiredSuffix)
    return t("providers.baseUrl.suffix", { suffix: endpoint.requiredSuffix });
  return t("providers.baseUrl.apiBase");
}

export function EndpointField({
  provider,
  draft,
  error,
  feedback,
  change,
  t,
}: {
  provider: Provider;
  draft: ProviderCredentialsDraft;
  error: ReturnType<ProviderCredentialsDraft["endpointError"]>;
  feedback?: string;
  change: (draft: ProviderCredentialsDraft) => void;
  t: Translate;
}) {
  const hintId = useId();
  const errorId = useId();
  const hint = pathHint(provider.endpoint, draft.endpointPath(provider), t);
  const issue =
    error === "required"
      ? t("providers.baseUrl.required")
      : error === "invalid"
        ? t("providers.baseUrl.invalid")
        : undefined;
  return (
    <FormField
      label={t(
        provider.requiresBaseUrl
          ? "providers.baseUrl.labelRequired"
          : "providers.baseUrl.label",
      )}
    >
      <TextField
        font="mono"
        type="text"
        required={provider.requiresBaseUrl}
        invalid={error !== undefined}
        aria-label={t("providers.baseUrl.aria", { provider: provider.id })}
        aria-describedby={
          [feedback, hint ? hintId : undefined, issue ? errorId : undefined]
            .filter(Boolean)
            .join(" ") || undefined
        }
        value={draft.baseURL(provider)}
        onChange={(event) => change(draft.withBaseURL(event.target.value))}
        placeholder={provider.endpoint?.example}
      />
      {hint && <span id={hintId}>{hint}</span>}
      {issue && <span id={errorId}>{issue}</span>}
    </FormField>
  );
}
