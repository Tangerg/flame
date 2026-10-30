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
  if (command?.phase === "unknown")
    return (
      <div role="alert" style={column}>
        <RecordBody>{t("command.uncertain")}</RecordBody>
        <RecordBody>{failureText(command.error)}</RecordBody>
        <Button onClick={() => void settings.recover(context, resource)}>
          {t("command.reconcile")}
        </Button>
      </div>
    );
  if (feedback?.kind === "error")
    return <ErrorMessage>{failureText(feedback.error)}</ErrorMessage>;
  if (feedback?.kind === "saved")
    return <p role="status">{t("providers.saved")}</p>;
  if (feedback?.kind === "tested") {
    if (feedback.outcome === "reachable")
      return <p role="status">{t("providers.connectionOk")}</p>;
    const key = {
      notConfigured: "providers.testOutcome.notConfigured",
      invalidCredentials: "rpcError.invalid_api_key",
      timedOut: "rpcError.timeout",
      failed: "providers.testOutcome.failed",
    }[feedback.outcome];
    return <ErrorMessage>{t(key)}</ErrorMessage>;
  }
  return null;
}

function ErrorMessage({ children }: { children: string }) {
  return (
    <div
      role="alert"
      style={{
        display: "flex",
        alignItems: "flex-start",
        gap: "var(--layout-content-gap)",
        whiteSpace: "normal",
        overflowWrap: "anywhere",
      }}
    >
      <Icon glyph={Alert} size="sm" />
      <span>{children}</span>
    </div>
  );
}
