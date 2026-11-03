import { useState, useSyncExternalStore } from "react";
import {
  failureText,
  type ComponentContext,
  type Translate,
} from "@flame/component-sdk";
import { Button, Icon, Metadata, RecordBody } from "@flame/ui";
import { Alert } from "@flame/icons/react";
import type { ReactNode } from "react";
import type { Read, MCPSettings } from "./settings";
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
  const command = state.commands.get(resource);
  const feedback = state.feedback.get(resource);
  if (command?.phase === "unknown")
    return (
      <div role="alert" style={column}>
        <RecordBody>{t("command.uncertain")}</RecordBody>
        <RecordBody>{failureText(command.error)}</RecordBody>
        <Button
          onClick={async () => {
            if (
              (await settings.recover(context, resource)) &&
              !context.signal.aborted
            )
              onSettled?.();
          }}
        >
          {t("command.reconcile")}
        </Button>
      </div>
    );
  if (feedback?.kind === "error")
    return <ErrorMessage>{failureText(feedback.error)}</ErrorMessage>;
  if (feedback?.kind === "tested")
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
  if (feedback?.kind === "imported")
    return (
      <div role="status">
        <RecordBody>{t("mcp.import.ok", { count: feedback.count })}</RecordBody>
      </div>
    );
  return null;
}

function ErrorMessage({ children }: { children: ReactNode }) {
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
