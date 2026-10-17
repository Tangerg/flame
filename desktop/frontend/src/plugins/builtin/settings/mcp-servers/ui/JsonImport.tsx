import { failureText } from "@/lib/rpcErrors";
import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useId, useRef, useState } from "react";
import { Icon, TextButton } from "@/ui";
import { Button, FormField, ModalDialog, TextArea, vocab } from "@flame/ui";
import { useCreateMCPServer } from "../application/mcpServerConfig";
import { notifyInfo } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { parseMcpImport } from "../application/mcpImport";
import { type as typeStep } from "@flame/ui/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

export function JsonImport() {
  const t = useT();
  const create = useCreateMCPServer();
  const field = useRef<HTMLTextAreaElement>(null);
  const errorId = useId();
  const [open, setOpen] = useState(false);
  const [text, setText] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | undefined>();

  const onImport = async () => {
    setBusy(true);
    setError(undefined);
    try {
      const { servers, renamed } = parseMcpImport(text);
      for (const server of servers) await create(server);
      notifyInfo(t("mcp.import.ok", { count: servers.length }), { source: "mcp" });
      if (renamed.length > 0) {
        notifyInfo(
          t("mcp.import.renamed", {
            names: renamed.map((entry) => `${entry.from} → ${entry.to}`).join(", "),
          }),
          { source: "mcp" },
        );
      }
      setText("");
      setOpen(false);
    } catch (err) {
      if (wasGenerationRetired(err)) return;
      setError(failureText(err, t("mcp.import.error")));
    } finally {
      setBusy(false);
    }
  };

  return (
    <>
      <TextButton onClick={() => setOpen(true)}>
        <Icon name="file-braces" size="sm" />
        {t("mcp.import")}
      </TextButton>
      <ModalDialog
        open={open}
        onOpenChange={setOpen}
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
            <Button type="button" variant="ghost" onClick={() => setOpen(false)}>
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
            onChange={(event) => setText(event.target.value)}
            rows={10}
            spellCheck={false}
            aria-describedby={error ? errorId : undefined}
            placeholder={
              '{"mcpServers": {"my-server": {"type": "streamableHttp", "url": "https://example.com/mcp"}}}'
            }
          />
        </FormField>
        {error && (
          <div
            id={errorId}
            role="alert"
            {...stylex.props(ss.inline, vocab.negative, typeStep.uiMd)}
          >
            <Icon name="alert" size="sm" />
            {error}
          </div>
        )}
      </ModalDialog>
    </>
  );
}
