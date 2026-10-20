import { failureText } from "@/lib/rpcErrors";
import { useId, useLayoutEffect, useRef, useState, type ReactNode } from "react";
import { Icon } from "@/ui/index";
import { SystemMessage } from "@/ui/index";
import { TextButton } from "@/ui/index";
import { Button, FormField, ModalDialog, TextArea } from "@flame/ui";
import {
  useMCPServerCommands,
  useMCPServerMutationMaterialGeneration,
} from "@/features/mcp/application/mcpServerConfig";
import { type MCPServerCommands } from "@/features/mcp/application/mcpServerMutationOwner";
import { notifyInfo } from "@/features/notifications/public/notifications";
import { Trans, useT } from "@/lib/i18n/index";
import { McpImportNameCollision, parseMcpImport } from "@/features/mcp/application/mcpImport";
const CONFIG_EXAMPLE = JSON.stringify(
  {
    mcpServers: {
      "my-server": { type: "streamableHttp", url: "https://example.com/mcp" },
    },
  },
  null,
  2,
);

export function JsonImport() {
  const currentCommands = useMCPServerCommands();
  const generation = useMCPServerMutationMaterialGeneration();
  return <JsonImportGeneration key={generation.toString()} currentCommands={currentCommands} />;
}

function JsonImportGeneration({ currentCommands }: { currentCommands: MCPServerCommands | null }) {
  const t = useT();
  const field = useRef<HTMLTextAreaElement>(null);
  const errorRef = useRef<HTMLDivElement>(null);
  const errorId = useId();
  const [draft, setDraft] = useState<{ commands: MCPServerCommands; text: string } | null>(null);
  const text = draft?.text ?? "";
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<{ message: ReactNode }>();
  useLayoutEffect(() => {
    if (error) errorRef.current?.scrollIntoView({ block: "nearest" });
  }, [error]);

  const onImport = async () => {
    if (!draft) return;
    setBusy(true);
    setError(undefined);
    try {
      const { servers, renamed } = parseMcpImport(text);
      const outcome = await draft.commands.importServers(servers);
      if (outcome.status === "retired") {
        notifyInfo(
          t("mcp.import.interrupted", { count: outcome.completed, total: outcome.total }),
          { source: "mcp" },
        );
        setDraft(null);
        return;
      }
      if (outcome.status === "failed") {
        const partial = t("mcp.import.partial", { count: outcome.completed, total: outcome.total });
        setError({ message: `${partial} ${failureText(outcome.error, t("mcp.import.error"))}` });
        return;
      }
      notifyInfo(t("mcp.import.ok", { count: servers.length }), { source: "mcp" });
      if (renamed.length > 0) {
        notifyInfo(
          t("mcp.import.renamed", {
            names: renamed.map((entry) => `${entry.from} → ${entry.to}`).join(", "),
          }),
          { source: "mcp" },
        );
      }
      setDraft(null);
    } catch (err) {
      if (err instanceof McpImportNameCollision) {
        setError({
          message: (
            <Trans
              i18nKey="mcp.import.nameCollision"
              components={{
                first: <span>{err.first}</span>,
                second: <span>{err.second}</span>,
                canonical: <span>{err.canonical}</span>,
              }}
            />
          ),
        });
        return;
      }
      setError({ message: failureText(err, t("mcp.import.error")) });
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <TextButton
        disabled={!currentCommands}
        pending={busy}
        onClick={() => {
          if (currentCommands) {
            setError(undefined);
            setDraft({ commands: currentCommands, text: "" });
          }
        }}
      >
        <Icon name="file-braces" size="sm" />
        {t("mcp.import")}
      </TextButton>
      <ModalDialog
        open={draft !== null}
        onOpenChange={(open) => {
          if (!open) setDraft(null);
        }}
        title={t("mcp.import")}
        description={t("mcp.import.hint")}
        closeLabel={t("common.close")}
        initialFocus={field}
        onSubmit={(event) => {
          event.preventDefault();
          if (!busy && text.trim()) void onImport();
        }}
        footer={
          <>
            <Button type="button" variant="ghost" onClick={() => setDraft(null)}>
              {t("common.cancel")}
            </Button>
            <Button type="submit" variant="primary" disabled={!text.trim()} pending={busy}>
              {busy ? t("mcp.importing") : t("mcp.import.confirm")}
            </Button>
          </>
        }
      >
        <FormField label={t("mcp.import.config")}>
          <TextArea
            ref={field}
            font="mono"
            invalid={error !== undefined}
            pending={busy}
            value={text}
            onChange={(event) =>
              setDraft((current) => current && { ...current, text: event.target.value })
            }
            rows={10}
            spellCheck={false}
            aria-describedby={error ? errorId : undefined}
            placeholder={CONFIG_EXAMPLE}
          />
        </FormField>
        {error && (
          <SystemMessage ref={errorRef} id={errorId} variant="error">
            {error.message}
          </SystemMessage>
        )}
      </ModalDialog>
    </>
  );
}
