import { useRef, useSyncExternalStore } from "react";
import {
  Button,
  CodeBlock,
  handoffFocus,
  ListRow,
  Metadata,
  PanelSection,
  RecordGroup,
  Toolbar,
} from "@flame/ui";
import { SystemMessage } from "@/ui/index";
import { useT } from "@/lib/i18n/index";
import { useCodeActionLabels } from "@/ui/index";
import { failureText } from "@/lib/rpcErrors";
import { type PackageOperations } from "@/features/plugins/adapters/packages";
import { type PluginCommandOutcome } from "@/features/plugins/adapters/pendingPluginCommands";

function CommandOutcome({ outcome }: { outcome: PluginCommandOutcome }) {
  const t = useT();
  const codeLabels = useCodeActionLabels();
  return (
    <div>
      <p role="status">{t(`packages.commands.${outcome.type}`, { id: outcome.key })}</p>
      {outcome.type === "confirmed" ? (
        <CodeBlock
          label={t("packages.commands.confirmed", { id: outcome.key })}
          labels={codeLabels}
        >
          {outcome.output}
        </CodeBlock>
      ) : outcome.type !== "retired" ? (
        <SystemMessage variant="warning">
          {failureText(outcome.error, t("packages.commands.failed"))}
        </SystemMessage>
      ) : null}
    </div>
  );
}

export function PendingCommands({
  operations,
}: {
  operations: Pick<PackageOperations, "pendingCommands" | "signal">;
}) {
  const t = useT();
  const commands = operations.pendingCommands;
  const panel = useRef<HTMLElement>(null);
  const snapshot = useSyncExternalStore(commands.subscribe, commands.snapshot);
  if (!snapshot) return null;
  const { read, observation } = snapshot;
  const checking = observation?.type === "checking";
  const disabled = checking || operations.signal.aborted;
  if (read.type === "ready" && read.commands.length === 0 && !observation) return null;
  return (
    <PanelSection
      ref={panel}
      tabIndex={-1}
      aria-label={t("packages.commands.title")}
      title={t("packages.commands.title")}
      description={t("packages.commands.explanation")}
      actions={
        read.type === "failed" && (
          <Button variant="soft" size="sm" disabled={disabled} onClick={commands.refresh}>
            {t("packages.commands.refresh")}
          </Button>
        )
      }
    >
      {checking && <p role="status">{t("packages.commands.checking", { id: observation.key })}</p>}
      {read.type === "failed" ? (
        <SystemMessage variant="warning">
          {failureText(read.error, t("packages.commands.failed"))}
        </SystemMessage>
      ) : (
        <>
          {!checking && !read.commands.length && <Metadata>{t("packages.commands.none")}</Metadata>}
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
                    disabled={disabled}
                    onClick={(event) => {
                      handoffFocus(event.currentTarget, panel.current);
                      void commands.observe(command.idempotencyKey, "recover");
                    }}
                  >
                    {t("packages.commands.recover")}
                  </Button>
                  <Button
                    disabled={disabled}
                    onClick={(event) => {
                      handoffFocus(event.currentTarget, panel.current);
                      void commands.observe(command.idempotencyKey, "retire");
                    }}
                  >
                    {t("packages.commands.retire")}
                  </Button>
                </Toolbar>
              </ListRow>
            ))}
          </RecordGroup>
        </>
      )}
      {observation && !checking && <CommandOutcome outcome={observation} />}
    </PanelSection>
  );
}
