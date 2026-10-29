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
import {
  McpImportError,
  parseMcpImport,
  type McpImportIssue,
} from "@/features/mcp/application/mcpImport";
const CONFIG_EXAMPLE = JSON.stringify(
  {
    mcpServers: {
      "my-server": { type: "streamableHttp", url: "https://example.com/mcp" },
    },
  },
  null,
  2,
);

const SERVER_ISSUE_KEYS: Record<Extract<McpImportIssue, { server: string }>["kind"], string> = {
  name: "mcp.import.invalidName",
  transport: "mcp.import.missingTransport",
  unsupportedTransport: "mcp.import.unsupportedTransport",
  headerConflict: "mcp.import.headerConflict",
  command: "mcp.import.missingCommand",
  url: "mcp.import.missingUrl",
  timeout: "mcp.import.invalidTimeout",
};

function McpImportIssueMessage({ issue }: { issue: McpImportIssue }) {
  const t = useT();
  switch (issue.kind) {
    case "json":
      return t("mcp.import.invalidJson");
    case "shape":
      return t("mcp.import.invalidShape");
    case "empty":
      return t("mcp.import.empty");
    default:
      return (
        <Trans
          i18nKey={SERVER_ISSUE_KEYS[issue.kind]}
          components={{
            server: <span>{issue.server}</span>,
            ...(issue.kind === "headerConflict" ? { header: <span>{issue.header}</span> } : {}),
          }}
        />
      );
  }
}

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
  const [error, setError] = useState<{ kind: "validation" | "operation"; message: ReactNode }>();
  const invalid = error?.kind === "validation";
  useLayoutEffect(() => {
    if (error) errorRef.current?.scrollIntoView({ block: "nearest" });
  }, [error]);

  const onImport = async () => {
    if (!draft) return;
    setError(undefined);
    let parsed: ReturnType<typeof parseMcpImport>;
    try {
      parsed = parseMcpImport(text);
    } catch (err) {
      if (err instanceof McpImportError) {
        setError({ kind: "validation", message: <McpImportIssueMessage issue={err.issue} /> });
        return;
      }
      setError({ kind: "operation", message: failureText(err, t("mcp.import.error")) });
      return;
    }
    const servers = parsed;
    setBusy(true);
    try {
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
        setError({
          kind: "operation",
          message: `${partial} ${failureText(outcome.error, t("mcp.import.error"))}`,
        });
        return;
      }
      notifyInfo(t("mcp.import.ok", { count: servers.length }), { source: "mcp" });
      setDraft(null);
    } catch (err) {
      setError({ kind: "operation", message: failureText(err, t("mcp.import.error")) });
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
            invalid={invalid}
            pending={busy}
            value={text}
            onChange={(event) => {
              setError(undefined);
              setDraft((current) => current && { ...current, text: event.target.value });
            }}
            rows={10}
            spellCheck={false}
            autoCapitalize="none"
            autoCorrect="off"
            aria-describedby={invalid ? errorId : undefined}
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
