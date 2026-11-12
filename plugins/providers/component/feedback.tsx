import { useState } from "react";
import {
  failureText,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import { Alert } from "@flame/icons/react";
import { Button, Icon, Metadata, RecordBody } from "@flame/ui";
import type { Command, Feedback, ProviderSettings } from "./settings";
const column = {
  display: "grid",
  minWidth: 0,
  gap: "var(--layout-field-gap)",
} as const;
export function ReadFailure({
  error,
  retained,
  retry,
  t,
}: {
  error: unknown;
  retained?: boolean;
  retry(): void;
  t: Translate;
}) {
  return (
    <div role="alert" style={column}>
      <RecordBody>{failureText(error)}</RecordBody>
      {retained && <Metadata>{t("read.retained")}</Metadata>}
      <Button onClick={retry}>{t("retry")}</Button>
    </div>
  );
}

export function CommandFeedback({
  context,
  settings,
  resource,
  command,
  feedback,
  t,
}: {
  context: ComponentContext;
  settings: ProviderSettings;
  resource: string;
  command?: Command;
  feedback?: Feedback;
  t: Translate;
}) {
  // Projects DOM focus so a focused recovery keeps its identity through settlement.
  const [focused, setFocused] = useState(false);
  const unknown = command?.phase === "unknown" ? command : undefined;
  const message = feedback && feedbackMessage(feedback, t);
  if (!unknown && !focused) return message ?? null;
  return (
    <div role={unknown ? "alert" : undefined} data-focus-unit style={column}>
      {unknown && <RecordBody>{t("command.uncertain")}</RecordBody>}
      {unknown && <RecordBody>{failureText(unknown.error)}</RecordBody>}
      {message}
      <Button
        pending={command?.phase === "running"}
        disabled={!command}
        focusableWhenDisabled
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        onClick={() => void settings.recover(context, resource)}
      >
        {t(command ? "command.reconcile" : "command.confirmed")}
      </Button>
    </div>
  );
}

function feedbackMessage(feedback: Feedback, t: Translate) {
  if (feedback.kind === "error")
    return <ErrorMessage>{failureText(feedback.error)}</ErrorMessage>;
  if (feedback.kind === "saved")
    return <p role="status">{t("providers.saved")}</p>;
  if (feedback.outcome === "reachable")
    return <p role="status">{t("providers.connectionOk")}</p>;
  const key = {
    notConfigured: "providers.testOutcome.notConfigured",
    invalidCredentials: "rpcError.invalid_credentials",
    timedOut: "rpcError.timeout",
    failed: "providers.testOutcome.failed",
  }[feedback.outcome];
  return <ErrorMessage>{t(key)}</ErrorMessage>;
}

function ErrorMessage({ children }: { children: string }) {
  return (
    <div
      role="alert"
      style={{
        display: "flex",
        alignItems: "flex-start",
        gap: "var(--layout-content-gap)",
        overflowWrap: "anywhere",
      }}
    >
      <Icon glyph={Alert} size="sm" />
      <RecordBody>{children}</RecordBody>
    </div>
  );
}
