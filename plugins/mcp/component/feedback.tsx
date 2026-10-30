import { useSyncExternalStore } from "react";
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
  t,
  retry,
}: {
  read: Read<T> | undefined;
  t: Translate;
  retry(): void;
}) {
  if (read?.phase === "failed")
    return (
      <div role="alert" style={column}>
        <RecordBody>{failureText(read.error)}</RecordBody>
        {read.data !== undefined && <Metadata>{t("read.retained")}</Metadata>}
        <Button onClick={retry}>{t("retry")}</Button>
      </div>
    );
  if (!read || (read.phase === "loading" && read.data === undefined))
    return <p role="status">{t("reading")}</p>;
  return null;
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
