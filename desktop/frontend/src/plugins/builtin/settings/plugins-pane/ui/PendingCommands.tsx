import { useLayoutEffect, useRef, useState } from "react";
import * as stylex from "@stylexjs/stylex";
import {
  OperationFailure,
  stringifyJSON,
  type PluginCommandRecovery,
} from "@flame/runtime-contract/client";
import { Button, Metadata, Toolbar } from "@flame/ui";
import { space } from "@flame/ui/tokens.stylex";
import { SystemMessage } from "@/ui";
import { useT } from "@/lib/i18n";
import { failureText } from "@/lib/rpcErrors";
import type { PackageOperations } from "../application/packages";

const styles = stylex.create({
  section: { padding: space.s4, display: "flex", flexDirection: "column", gap: space.s3 },
  output: { whiteSpace: "pre-wrap", overflowWrap: "anywhere", maxHeight: 240, overflow: "auto" },
});

type Outcome =
  | { type: "confirmed"; key: string; output: string }
  | { type: "refused" | "unknown"; key: string; reason: string };

export function PendingCommands({ operations }: { operations: PackageOperations }) {
  const t = useT();
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

  async function recover(command: PluginCommandRecovery) {
    if (current?.checking || operations.signal.aborted) return;
    controller.current?.value.abort();
    const owned = new AbortController();
    controller.current = { operations, value: owned };
    const signal = AbortSignal.any([operations.signal, owned.signal]);
    setObservation({ operations, checking: command.idempotencyKey });
    try {
      const output = await operations.track(
        command.recover((value) => stringifyJSON(value), signal),
      );
      if (!signal.aborted)
        setObservation({
          operations,
          outcome: { type: "confirmed", key: command.idempotencyKey, output },
        });
    } catch (error) {
      if (!signal.aborted)
        setObservation({
          operations,
          outcome: {
            type: error instanceof OperationFailure ? "refused" : "unknown",
            key: command.idempotencyKey,
            reason: failureText(error, t("packages.commands.failed")),
          },
        });
    }
  }

  return (
    <section aria-label={t("packages.commands.title")} {...stylex.props(styles.section)}>
      <Toolbar label={t("packages.commands.title")}>
        <h3>{t("packages.commands.title")}</h3>
        <Button
          disabled={!!current?.checking || operations.signal.aborted}
          onClick={() => setObservation({ operations, outcome: current?.outcome })}
        >
          {t("packages.commands.refresh")}
        </Button>
      </Toolbar>
      <Metadata>{t("packages.commands.explanation")}</Metadata>
      {current?.checking && (
        <p role="status">{t("packages.commands.checking", { id: current.checking })}</p>
      )}
      {read.type === "failed" ? (
        <SystemMessage variant="warning">{read.reason}</SystemMessage>
      ) : (
        <>
          {!current?.checking && !read.commands.length && (
            <Metadata>{t("packages.commands.none")}</Metadata>
          )}
          {read.commands.map((command) => (
            <div key={command.idempotencyKey} role="group" aria-label={command.idempotencyKey}>
              <Metadata>{command.idempotencyKey}</Metadata>
              <Metadata>
                {command.binding.method} · {command.binding.installationId} ·{" "}
                {command.binding.digest}
              </Metadata>
              <Button
                disabled={!!current?.checking || operations.signal.aborted}
                onClick={() => void recover(command)}
              >
                {t("packages.commands.recover")}
              </Button>
            </div>
          ))}
        </>
      )}
      {current?.outcome && (
        <div>
          <p role="status">
            {t(`packages.commands.${current.outcome.type}`, { id: current.outcome.key })}
          </p>
          {current.outcome.type === "confirmed" ? (
            <pre {...stylex.props(styles.output)}>{current.outcome.output}</pre>
          ) : (
            <SystemMessage variant="warning">{current.outcome.reason}</SystemMessage>
          )}
        </div>
      )}
    </section>
  );
}
