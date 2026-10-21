import * as stylex from "@stylexjs/stylex";
import { useRef, useState } from "react";
import { StatusDot } from "@/ui/index";
import { Button, FormActions, FormField, isImeKey, TextField, vocab } from "@flame/ui";
import { useT, type Translate } from "@/lib/i18n/index";
import {
  applyRuntimeEndpoint,
  currentRuntimeEndpoint,
  resetRuntimeEndpoint,
  defaultRuntimeEndpoint,
  hasRuntimeAccessToken,
  type RuntimeEndpointRejection,
} from "@/features/connection/public/endpoint";
import {
  refreshRuntimeServiceStatus,
  useRuntimeServiceStatus,
} from "@/features/connection/public/serviceStatus";
import { type RuntimeServicePhase } from "@/features/connection/public/serviceStatus";
import { SettingRow } from "@/ui/settings/SettingRow";
import { SettingsGroup } from "@/ui/settings/SettingsGroup";
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
  status: {
    marginTop: space.s1,
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
    fontFamily: "var(--font-mono)",
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

const STATUS_KEY: Record<RuntimeServicePhase, string> = {
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
    case "invalid_token":
      return translate("connection.error.token");
  }
}

export function ConnectionPane() {
  const t = useT();
  const initial = currentRuntimeEndpoint();
  const defaultEndpoint = defaultRuntimeEndpoint();
  const service = useRuntimeServiceStatus();
  const [url, setUrl] = useState(initial);
  const [token, setToken] = useState("");
  const [tokenEdited, setTokenEdited] = useState(false);
  const [error, setError] = useState<RuntimeEndpointRejection | null>(null);
  const [refreshing, setRefreshing] = useState(false);
  const tokenField = useRef<HTMLInputElement>(null);

  const trimmed = url.trim();
  const dirty = trimmed !== initial.trim() || tokenEdited;
  const isDefault = trimmed === defaultEndpoint;

  const apply = (): boolean => {
    const result = applyRuntimeEndpoint(url, tokenEdited ? token : undefined);
    if (result.kind === "rejected") {
      setError(result.reason);
      return false;
    }
    setUrl(result.endpoint);
    setError(null);
    setToken("");
    setTokenEdited(false);
    return true;
  };

  const reset = () => {
    const result = resetRuntimeEndpoint();
    if (result.kind === "rejected") {
      setError(result.reason);
      return;
    }
    setUrl(result.endpoint);
    setToken("");
    setTokenEdited(false);
    setError(null);
  };

  const discardToken = () => {
    setToken("");
    setTokenEdited(false);
    setError((current) => (current === "invalid_token" ? null : current));
    tokenField.current?.focus();
  };

  const tokenState = tokenEdited
    ? "settings.connection.tokenPending"
    : hasRuntimeAccessToken()
      ? "settings.connection.tokenSet"
      : "settings.connection.tokenNotSet";

  const refresh = async () => {
    setRefreshing(true);
    try {
      await refreshRuntimeServiceStatus();
    } finally {
      setRefreshing(false);
    }
  };

  const unhealthyChecks = Object.entries(service.observation?.checks ?? {}).filter(
    ([, health]) => health !== "ready",
  );

  return (
    <SettingsGroup>
      <SettingRow
        label={t("settings.connection.title")}
        sub={t("settings.connection.sub")}
        align="stacked"
      >
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
              spellCheck={false}
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
              value={token}
              onChange={(event) => {
                setToken(event.target.value);
                setTokenEdited(true);
              }}
              placeholder={t("settings.connection.tokenPlaceholder")}
              spellCheck={false}
            />
          </FormField>
          <p
            id="runtime-token-state"
            aria-live="polite"
            {...stylex.props(vocab.muted, typeStep.uiSm)}
          >
            {t(tokenState)}
          </p>
          <p id="runtime-token-hint" {...stylex.props(vocab.muted, typeStep.uiSm)}>
            {t("settings.connection.tokenHint")}
          </p>
          <FormActions
            label={t("settings.connection.apply")}
            auxiliaryAction={
              <>
                <Button
                  type="button"
                  variant="ghost"
                  size="md"
                  disabled={isDefault}
                  onClick={reset}
                  className={stylex.props(vocab.hold).className}
                >
                  {t("settings.connection.reset")}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  disabled={!token && !hasRuntimeAccessToken()}
                  onClick={() => {
                    setToken("");
                    setTokenEdited(true);
                  }}
                  styles={vocab.hold}
                >
                  {t("settings.connection.clearToken")}
                </Button>
                <Button
                  type="button"
                  variant="ghost"
                  disabled={!tokenEdited}
                  onClick={discardToken}
                  styles={vocab.hold}
                >
                  {t("settings.connection.discardToken")}
                </Button>
              </>
            }
          >
            <Button
              type="button"
              variant="primary"
              size="md"
              disabled={!dirty}
              onClick={() => void apply()}
              className={stylex.props(vocab.hold).className}
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
                pending={
                  refreshing || service.phase === "checking" || service.phase === "reconnecting"
                }
                onClick={() => void refresh()}
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
                    <dd {...stylex.props(cp.checks)}>
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
        </div>
      </SettingRow>
    </SettingsGroup>
  );
}
