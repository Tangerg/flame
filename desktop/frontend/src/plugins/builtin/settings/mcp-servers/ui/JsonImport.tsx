import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useState } from "react";
import { Icon, PillButton, Surface, TextArea, TextButton } from "@/ui";
import { useCreateMCPServer } from "../application/mcpServerConfig";
import { notifyInfo } from "@/plugins/sdk";
import { useT } from "@/lib/i18n";
import { parseMcpImport } from "../application/mcpImport";
import { space, type as typeStep } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

const ji = stylex.create({
  form: { display: "flex", flexDirection: "column", gap: space.s2_5 },
});

export function JsonImport() {
  const t = useT();
  const create = useCreateMCPServer();
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
      setError(err instanceof Error ? err.message : t("mcp.import.error"));
    } finally {
      setBusy(false);
    }
  };

  if (!open) {
    return (
      <TextButton onClick={() => setOpen(true)}>
        <Icon name="download" size="sm" />
        {t("mcp.import")}
      </TextButton>
    );
  }
  return (
    <Surface className={stylex.props(ji.form).className}>
      <span {...stylex.props(ss.muted, typeStep.uiMd)}>{t("mcp.import.hint")}</span>
      <TextArea
        size="sm"
        invalid={error !== undefined}
        value={text}
        onChange={(event) => setText(event.target.value)}
        rows={6}
        spellCheck={false}
        aria-label={t("mcp.import.hint")}
        placeholder={
          '{"mcpServers": {"my-server": {"type": "streamableHttp", "url": "https://example.com/mcp"}}}'
        }
      />
      {error && (
        <span {...stylex.props(ss.inline, ss.negative, typeStep.uiMd)}>
          <Icon name="alert" size="sm" />
          <span {...stylex.props(ss.truncate)} title={error}>
            {error}
          </span>
        </span>
      )}
      <div {...stylex.props(ss.line)}>
        <PillButton
          variant="accent"
          size="sm"
          disabled={!text.trim() || busy}
          onClick={() => void onImport()}
        >
          {busy ? t("mcp.importing") : t("mcp.import.confirm")}
        </PillButton>
        <PillButton variant="outlined" size="sm" onClick={() => setOpen(false)}>
          {t("common.cancel")}
        </PillButton>
      </div>
    </Surface>
  );
}
