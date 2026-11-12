import { useState, useSyncExternalStore } from "react";
import {
  failureText,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import { Button, Icon, Metadata, RecordBody } from "@flame/ui";
import { Alert } from "@flame/icons/react";
import type { ReactNode } from "react";
import type { Feedback, Read, MCPSettings } from "./settings";
const column = {
  display: "grid",
  minWidth: 0,
  gap: "var(--layout-content-gap)",
} as const;
export function ReadStatus<T>({
  read,
  title,
  t,
  retry,
}: {
  read: Read<T> | undefined;
  title?: string;
  t: Translate;
  retry(): void;
}) {
  // Projects DOM focus so a focused action keeps its identity across read phases.
  const [focused, setFocused] = useState(false);
  const failure = read?.phase === "failed" ? read : undefined;
  const pending = !read || read.phase === "loading";
  if (!failure && !focused)
    return pending && read?.data === undefined ? (
      <p role="status">{t("reading")}</p>
    ) : null;
  return (
    <div
      role={failure ? "alert" : undefined}
      data-focus-unit={failure ? true : undefined}
      style={column}
    >
      {failure && title && <strong>{title}</strong>}
      {failure && <RecordBody>{failureText(failure.error)}</RecordBody>}
      {failure?.data !== undefined && <Metadata>{t("read.retained")}</Metadata>}
      <Button
        pending={pending}
        onFocus={() => setFocused(true)}
        onBlur={() => setFocused(false)}
        onClick={retry}
      >
        {t(failure ? "retry" : pending ? "reading" : "refresh")}
      </Button>
    </div>
  );
}
export function CommandFeedback({
  context,
  settings,
  resource,
  t,
  onSettled,
}: {
  context: ComponentContext;
  settings: MCPSettings;
  resource: string;
  t: Translate;
  onSettled?(): void;
}) {
  const state = useSyncExternalStore(settings.subscribe, settings.getSnapshot);
  // Projects DOM focus so a focused recovery keeps its identity through settlement.
  const [focused, setFocused] = useState(false);
  const command = state.commands.get(resource);
  const unknown = command?.phase === "unknown" ? command : undefined;
  const feedback = state.feedback.get(resource);
  const message = feedback && feedbackMessage(feedback, t);
  const recovering = unknown !== undefined || focused;
  if (!recovering && feedback?.kind !== "error") return message ?? null;
  return (
    <div role={unknown ? "alert" : undefined} data-focus-unit style={column}>
      {unknown && <RecordBody>{t("command.uncertain")}</RecordBody>}
      {unknown && <RecordBody>{failureText(unknown.error)}</RecordBody>}
      {message}
      {recovering && (
        <Button
          pending={command?.phase === "running"}
          disabled={!command}
          focusableWhenDisabled
          onFocus={() => setFocused(true)}
          onBlur={() => setFocused(false)}
          onClick={async () => {
            if (
              (await settings.recover(context, resource)) &&
              !context.signal.aborted
            )
              onSettled?.();
          }}
        >
          {t(command ? "command.reconcile" : "command.confirmed")}
        </Button>
      )}
    </div>
  );
}

function feedbackMessage(feedback: Feedback, t: Translate) {
  if (feedback.kind === "error")
    return <ErrorMessage>{failureText(feedback.error)}</ErrorMessage>;
  if (feedback.kind === "tested")
    return (
      <p role={feedback.outcome === "reachable" ? "status" : "alert"}>
        {t(
          {
            reachable: "mcp.connectionOk",
            authorizationRequired: "mcpStatus.mcp_authorization_required",
            timedOut: "mcp.testOutcome.timedOut",
            failed: "mcp.testOutcome.failed",
          }[feedback.outcome],
        )}
      </p>
    );
  return (
    <div role="status">
      <RecordBody>{t("mcp.import.ok", { count: feedback.count })}</RecordBody>
    </div>
  );
}

function ErrorMessage({ children }: { children: ReactNode }) {
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
