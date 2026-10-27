import { useLayoutEffect, useRef, useState } from "react";
import { stringifyJSON } from "@qimu/sdk/json";
import { OperationFailure, isErrorType, type PluginCommandRecovery } from "@flame/runtime-client";
import {
  Button,
  CodeBlock,
  ListRow,
  Metadata,
  PanelSection,
  RecordGroup,
  RecordMeta,
  Toolbar,
} from "@flame/ui";
import { SystemMessage } from "@/ui/index";
import { useT } from "@/lib/i18n/index";
import { useCodeActionLabels } from "@/ui/index";
import { failureText } from "@/lib/rpcErrors";
import { type PackageOperations } from "@/features/plugins/adapters/packages";
type Outcome =
  | { type: "confirmed"; key: string; output: string }
  | { type: "retired"; key: string }
  | { type: "refused" | "unknown"; key: string; reason: string };

export function PendingCommands({ operations }: { operations: PackageOperations }) {
  const t = useT();
  const codeLabels = useCodeActionLabels();
  const [observation, setObservation] = useState<{
    operations: PackageOperations;
    checking?: string;
    outcome?: Outcome;
  }>();
  const current = observation?.operations === operations ? observation : undefined;
  const controller = useRef<{ operations: PackageOperations; value: AbortController }>(undefined);
  useLayoutEffect(
    () => () => {
      if (controller.current?.operations === operations) controller.current.value.abort();
    },
    [operations],
  );

  let read:
    { type: "ready"; commands: PluginCommandRecovery[] } | { type: "failed"; reason: string };
  try {
    read = { type: "ready", commands: operations.pendingCommands() };
  } catch (error) {
    read = { type: "failed", reason: failureText(error, t("packages.commands.failed")) };
  }
  const empty =
    read.type === "ready" && read.commands.length === 0 && !current?.checking && !current?.outcome;

  async function observe(command: PluginCommandRecovery, action: "recover" | "retire") {
    if (current?.checking || operations.signal.aborted) return;
    controller.current?.value.abort();
    const owned = new AbortController();
    controller.current = { operations, value: owned };
    const signal = AbortSignal.any([operations.signal, owned.signal]);
    setObservation({ operations, checking: command.idempotencyKey });
    try {
      let outcome: Outcome;
      if (action === "retire") {
        await operations.track(() => command.retire(signal));
        outcome = { type: "retired", key: command.idempotencyKey };
      } else {
        const output = await operations.track(() =>
          command.recover((value) => stringifyJSON(value), signal),
        );
        outcome = { type: "confirmed", key: command.idempotencyKey, output };
      }
      if (!signal.aborted)
        setObservation({
          operations,
          outcome,
        });
    } catch (error) {
      if (!signal.aborted)
        setObservation({
          operations,
          outcome: isErrorType(error, "plugin_command_retired")
            ? { type: "retired", key: command.idempotencyKey }
            : {
                type: error instanceof OperationFailure ? "refused" : "unknown",
                key: command.idempotencyKey,
                reason: failureText(error, t("packages.commands.failed")),
              },
        });
    }
  }

  return (
    <PanelSection
      aria-label={t("packages.commands.title")}
      title={
        empty ? (
          <RecordMeta>{t("packages.commands.none")}</RecordMeta>
        ) : (
          t("packages.commands.title")
        )
      }
      description={empty ? undefined : t("packages.commands.explanation")}
      actions={
        <Button
          variant="soft"
          size="sm"
          disabled={!!current?.checking || operations.signal.aborted}
          onClick={() => setObservation({ operations, outcome: current?.outcome })}
        >
          {t("packages.commands.refresh")}
        </Button>
      }
    >
      {current?.checking && (
        <p role="status">{t("packages.commands.checking", { id: current.checking })}</p>
      )}
      {read.type === "failed" ? (
        <SystemMessage variant="warning">{read.reason}</SystemMessage>
      ) : (
        <>
          {!empty && !current?.checking && !read.commands.length && (
            <Metadata>{t("packages.commands.none")}</Metadata>
          )}
          <RecordGroup variant="list" hidden={!read.commands.length}>
            {read.commands.map((command) => (
              <ListRow
                key={command.idempotencyKey}
                role="group"
                aria-label={command.idempotencyKey}
              >
                <Metadata>{command.idempotencyKey}</Metadata>
                <Metadata>
                  {command.binding.method} · {command.binding.installationId} ·{" "}
                  {command.binding.digest}
                </Metadata>
                <Toolbar label={command.idempotencyKey}>
                  <Button
                    disabled={!!current?.checking || operations.signal.aborted}
                    onClick={() => void observe(command, "recover")}
                  >
                    {t("packages.commands.recover")}
                  </Button>
                  <Button
                    disabled={!!current?.checking || operations.signal.aborted}
                    onClick={() => void observe(command, "retire")}
                  >
                    {t("packages.commands.retire")}
                  </Button>
                </Toolbar>
              </ListRow>
            ))}
          </RecordGroup>
        </>
      )}
      {current?.outcome && (
        <div>
          <p role="status">
            {t(`packages.commands.${current.outcome.type}`, { id: current.outcome.key })}
          </p>
          {current.outcome.type === "confirmed" ? (
            <CodeBlock
              label={t("packages.commands.confirmed", { id: current.outcome.key })}
              labels={codeLabels}
            >
              {current.outcome.output}
            </CodeBlock>
          ) : current.outcome.type !== "retired" ? (
            <SystemMessage variant="warning">{current.outcome.reason}</SystemMessage>
          ) : null}
        </div>
      )}
    </PanelSection>
  );
}
