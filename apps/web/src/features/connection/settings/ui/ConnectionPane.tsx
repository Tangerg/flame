import * as stylex from "@stylexjs/stylex";
import { useRef, useState } from "react";
import { StatusDot } from "@/ui/index";
import {
  Button,
  FormActions,
  FormField,
  isImeKey,
  PanelSection,
  TextField,
  vocab,
} from "@flame/ui";
import { useT, type Translate, type MessageKey } from "@/lib/i18n/index";
import { useRuntime, type RuntimeEndpointRejection } from "@/features/connection/public/endpoint";
import { useRuntimeServiceStatus } from "@/features/connection/public/serviceStatus";
import { type RuntimeServicePhase } from "@/features/connection/public/serviceStatus";
import {
  color,
  face,
  layout,
  radius,
  space,
  surface,
  type as typeStep,
} from "@flame/ui/tokens.stylex";
import { settingStyles as ss } from "@/ui/settings/settingStyles";
const cp = stylex.create({
  form: { display: "grid", gap: layout.fieldGap, minWidth: 0 },
  notes: { display: "grid", minWidth: 0 },
  pending: { margin: 0, marginBottom: { default: space.s1, ":empty": 0 } },
  // The fill hangs like the fields above, so its text starts on the labels' edge.
  status: {
    marginTop: space.s1,
    marginInlineStart: `calc(-1 * ${space.s3})`,
    borderRadius: radius.card,
    backgroundColor: surface.sunken,
    paddingInline: space.s3,
    paddingBlock: space.s2_5,
  },
  facts: {
    marginTop: space.s2,
    display: "grid",
    gridTemplateColumns: "auto minmax(0, 1fr)",
    columnGap: space.s3,
    rowGap: space.s1,
  },
  checks: {
    display: "flex",
    minWidth: 0,
    flexWrap: "wrap",
    columnGap: space.s2,
    color: color.warning,
  },
  detail: { marginTop: space.s2, overflowWrap: "break-word", color: color.negative },
});

const STATUS_TONE: Record<RuntimeServicePhase, "ok" | "running" | "waiting" | "err"> = {
  checking: "running",
  reconnecting: "running",
  ready: "ok",
  degraded: "waiting",
  unhealthy: "err",
  unavailable: "err",
};

const STATUS_KEY: Record<RuntimeServicePhase, MessageKey> = {
  checking: "settings.connection.status.checking",
  reconnecting: "settings.connection.status.reconnecting",
  ready: "settings.connection.status.ready",
  degraded: "settings.connection.status.degraded",
  unhealthy: "settings.connection.status.unhealthy",
  unavailable: "settings.connection.status.unavailable",
};

function rejectionMessage(reason: RuntimeEndpointRejection, translate: Translate): string {
  switch (reason) {
    case "invalid_url":
      return translate("connection.error.invalidUrl");
    case "unsupported_scheme":
      return translate("connection.error.urlScheme");
    case "insecure_bearer":
      return translate("connection.error.insecureBearer");
    case "storage_failed":
      return translate("connection.error.storage");
    case "invalid_token":
      return translate("connection.error.token");
  }
}

function RuntimeServiceStatus({
  service,
  refreshing,
  onRefresh,
}: {
  service: ReturnType<typeof useRuntimeServiceStatus>;
  refreshing: boolean;
  onRefresh(): void;
}) {
  const t = useT();
  const unhealthyChecks = Object.entries(service.observation?.checks ?? {}).filter(
    ([, health]) => health !== "ready",
  );
  return (
    <div {...stylex.props(cp.status)}>
      <div {...stylex.props(ss.split)} aria-live="polite">
        <div {...stylex.props(vocab.line, vocab.min, vocab.muted, typeStep.uiMd)}>
          <StatusDot tone={STATUS_TONE[service.phase]} />
          <span>{t(STATUS_KEY[service.phase])}</span>
        </div>
        <Button
          type="button"
          variant="soft"
          size="sm"
          pending={refreshing || service.phase === "checking" || service.phase === "reconnecting"}
          onClick={onRefresh}
        >
          {t("settings.connection.status.refresh")}
        </Button>
      </div>
      {service.observation ? (
        <dl {...stylex.props(cp.facts, typeStep.uiSm)}>
          <dt {...stylex.props(vocab.faint)}>{t("settings.connection.status.server")}</dt>
          <dd {...stylex.props(vocab.truncate, vocab.muted, face.mono)}>
            {service.observation.server.name} {service.observation.server.version}
          </dd>
          <dt {...stylex.props(vocab.faint)}>{t("settings.connection.status.protocol")}</dt>
          <dd {...stylex.props(vocab.truncate, vocab.muted, face.mono)}>
            {service.observation.protocolVersion}
          </dd>
          {unhealthyChecks.length > 0 ? (
            <>
              <dt {...stylex.props(vocab.faint)}>{t("settings.connection.status.checks")}</dt>
              <dd {...stylex.props(cp.checks, face.mono)}>
                {unhealthyChecks.map(([name, health]) => (
                  <span key={name}>
                    {name} <span {...stylex.props(vocab.faint)}>{t(STATUS_KEY[health])}</span>
                  </span>
                ))}
              </dd>
            </>
          ) : null}
        </dl>
      ) : null}
      {service.failure ? (
        <p {...stylex.props(cp.detail, typeStep.uiSm)}>
          {service.failure.reason === "timeout"
            ? t("settings.connection.status.timeout")
            : service.failure.detail}
        </p>
      ) : null}
    </div>
  );
}

export function ConnectionPane() {
  const runtime = useRuntime();
  const {
    applyRuntimeEndpoint,
    currentRuntimeEndpoint,
    resetRuntimeEndpoint,
    defaultRuntimeEndpoint,
    hasRuntimeAccessToken,
  } = runtime.endpoint;
  const t = useT();
  const initial = currentRuntimeEndpoint();
  const defaultEndpoint = defaultRuntimeEndpoint();
  const service = useRuntimeServiceStatus();
  const [url, setUrl] = useState(initial);
  // null keeps the stored token; a string, even empty, replaces it on apply.
  const [tokenDraft, setTokenDraft] = useState<string | null>(null);
  const [error, setError] = useState<RuntimeEndpointRejection | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const tokenField = useRef<HTMLInputElement>(null);

  const trimmed = url.trim();
  const dirty = trimmed !== initial.trim() || tokenDraft !== null;
  const isDefault = trimmed === defaultEndpoint;

  const apply = (): boolean => {
    const result = applyRuntimeEndpoint(url, tokenDraft ?? undefined);
    if (result.kind === "rejected") {
      setError(result.reason);
      return false;
    }
    setUrl(result.endpoint);
    setError(null);
    setTokenDraft(null);
    return true;
  };

  const reset = () => {
    const result = resetRuntimeEndpoint();
    if (result.kind === "rejected") {
      setError(result.reason);
      return;
    }
    setUrl(result.endpoint);
    setTokenDraft(null);
    setError(null);
  };

  const discardToken = () => {
    setTokenDraft(null);
    setError((current) => (current === "invalid_token" ? null : current));
    tokenField.current?.focus();
  };

  const tokenStored = hasRuntimeAccessToken();

  const refresh = async () => {
    setRefreshing(true);
    try {
      await runtime.status.refresh();
    } finally {
      setRefreshing(false);
    }
  };

  return (
    <PanelSection title={t("settings.connection.title")}>
      <div {...stylex.props(cp.form)}>
        <FormField label={t("settings.connection.url")}>
          <TextField
            font="mono"
            id="runtime-base-url"
            type="text"
            invalid={error !== null && error !== "invalid_token"}
            aria-label={t("settings.connection.url")}
            value={url}
            onChange={(e) => setUrl(e.target.value)}
            aria-describedby={
              error && error !== "invalid_token" ? "runtime-connection-error" : undefined
            }
            onKeyDown={(e) => {
              if (e.key !== "Enter" || isImeKey(e.nativeEvent)) return;
              e.preventDefault();
              if (apply()) e.currentTarget.blur();
            }}
            placeholder={defaultEndpoint}
          />
        </FormField>
        <FormField label={t("settings.connection.token")}>
          <TextField
            ref={tokenField}
            id="runtime-local-token"
            type="password"
            autoComplete="off"
            aria-label={t("settings.connection.token")}
            invalid={error === "invalid_token"}
            aria-describedby={[
              "runtime-token-state",
              "runtime-token-hint",
              error === "invalid_token" ? "runtime-connection-error" : null,
            ]
              .filter(Boolean)
              .join(" ")}
            value={tokenDraft ?? ""}
            onChange={(event) => {
              setTokenDraft(event.target.value);
            }}
            // The masked field cannot show a stored token; its placeholder says one exists.
            placeholder={
              tokenDraft === null && tokenStored ? t("settings.connection.tokenReplace") : undefined
            }
            spellCheck={false}
          />
        </FormField>
        <div {...stylex.props(cp.notes)}>
          {/* Stays mounted so a pending change is announced; empty, it takes no space. */}
          <p
            id="runtime-token-state"
            aria-live="polite"
            {...stylex.props(vocab.muted, typeStep.uiSm, cp.pending)}
          >
            {tokenDraft !== null && t("settings.connection.tokenPending")}
          </p>
          <p id="runtime-token-hint" {...stylex.props(vocab.muted, typeStep.uiSm)}>
            {t("settings.connection.tokenHint")}
          </p>
        </div>
        <FormActions
          label={t("settings.connection.apply")}
          auxiliaryAction={
            <>
              <Button
                type="button"
                variant="ghost"
                size="md"
                disabled={isDefault}
                focusableWhenDisabled
                onClick={reset}
                styles={vocab.hold}
              >
                {t("settings.connection.reset")}
              </Button>
              <Button
                type="button"
                variant="ghost"
                disabled={!tokenDraft && !hasRuntimeAccessToken()}
                focusableWhenDisabled
                onClick={() => {
                  setTokenDraft("");
                }}
                styles={vocab.hold}
              >
                {t("settings.connection.clearToken")}
              </Button>
              {tokenDraft !== null && (
                <Button type="button" variant="ghost" onClick={discardToken} styles={vocab.hold}>
                  {t("settings.connection.discardToken")}
                </Button>
              )}
            </>
          }
        >
          <Button
            type="button"
            variant="primary"
            size="md"
            disabled={!dirty}
            focusableWhenDisabled
            onClick={() => void apply()}
            styles={vocab.hold}
          >
            {t("settings.connection.apply")}
          </Button>
        </FormActions>
        {error ? (
          <div
            id="runtime-connection-error"
            role="alert"
            {...stylex.props(vocab.lineTight, vocab.negative, typeStep.uiSm)}
          >
            <StatusDot tone="err" />
            <span>{rejectionMessage(error, t)}</span>
          </div>
        ) : null}
        <RuntimeServiceStatus
          service={service}
          refreshing={refreshing}
          onRefresh={() => void refresh()}
        />
      </div>
    </PanelSection>
  );
}
