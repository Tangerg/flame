import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useEffect, useId, useRef, useState } from "react";
import { IconButton, PillButton, StatusDot, Switch, Tag, vocab } from "@/ui";
import {
  type MCPServerSettings,
  type MCPTransport,
  useAuthorizeMCPServer,
  useSetMCPServerEnabled,
} from "../application/mcpServerConfig";
import { notifyError } from "@/plugins/sdk";
import type { DotTone } from "@/lib/tone";
import { useT } from "@/lib/i18n";
import { ServerForm } from "./ServerForm";
import { face, space, type as typeStep } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

const STATUS_TONE: Record<MCPServerSettings["status"], DotTone> = {
  disabled: "idle",
  connected: "ok",
  connecting: "running",
  needsAuth: "waiting",
  failed: "err",
  disconnected: "idle",
};

function TransportBadge({ transport }: { transport: MCPTransport }) {
  return <Tag size="sm">{transport}</Tag>;
}

const sr = stylex.create({
  head: {
    display: "grid",
    gridTemplateColumns: "auto minmax(0, 1fr) auto",
    alignItems: "center",
    gap: space.s3,
  },
  actions: { display: "flex", alignItems: "center", gap: space.s2_5 },
});

export function ServerRow({ server }: { server: MCPServerSettings }) {
  const t = useT();
  const setEnabled = useSetMCPServerEnabled();
  const authorize = useAuthorizeMCPServer();
  const [editing, setEditing] = useState(false);
  const panelId = useId();
  const [signingIn, setSigningIn] = useState(false);
  const authorizationController = useRef<AbortController | null>(null);

  useEffect(
    () => () => {
      const controller = authorizationController.current;
      authorizationController.current = null;
      controller?.abort();
    },
    [],
  );

  const onToggle = async (enabled: boolean) => {
    try {
      await setEnabled(server.name, enabled);
    } catch (err) {
      if (wasGenerationRetired(err)) return;
      notifyError(err instanceof Error ? err.message : t("mcp.error.toggle"), { source: "mcp" });
    }
  };

  const onSignIn = async () => {
    const controller = new AbortController();
    authorizationController.current?.abort();
    authorizationController.current = controller;
    setSigningIn(true);
    try {
      await authorize(server.name, controller.signal);
    } catch (err) {
      if (controller.signal.aborted || wasGenerationRetired(err)) return;
      notifyError(err instanceof Error ? err.message : t("mcp.error.signIn"), { source: "mcp" });
    } finally {
      if (authorizationController.current === controller) {
        authorizationController.current = null;
        setSigningIn(false);
      }
    }
  };

  const tone = STATUS_TONE[server.status];
  const active = server.status === "connected";

  return (
    <div {...stylex.props(ss.hoverRow)}>
      <div {...stylex.props(sr.head)}>
        <StatusDot tone={tone} />
        <div {...stylex.props(vocab.line, vocab.min)}>
          <span {...stylex.props(vocab.truncate, ss.label, typeStep.uiMd)} title={server.name}>
            {server.name}
          </span>
          <TransportBadge transport={server.type} />
          {server.status === "failed" && server.errorDetail && (
            <span
              {...stylex.props(vocab.truncate, vocab.negative, typeStep.uiMd)}
              title={server.errorDetail}
            >
              {server.errorDetail}
            </span>
          )}
        </div>
        <div {...stylex.props(sr.actions)}>
          {active && (
            <span {...stylex.props(vocab.muted, typeStep.uiMd, face.mono)}>
              {t("mcp.toolCount", { count: server.toolCount ?? 0 })}
            </span>
          )}
          {(server.status === "needsAuth" || signingIn) && (
            <PillButton
              variant="accent"
              size="sm"
              disabled={signingIn}
              onClick={() => void onSignIn()}
            >
              {t(signingIn ? "mcp.signingIn" : "mcp.signIn")}
            </PillButton>
          )}
          <Switch
            checked={server.enabled}
            onCheckedChange={(value) => void onToggle(value)}
            ariaLabel={t("mcp.enable.aria", { server: server.name })}
          />
          <IconButton
            icon="edit"
            size="sm"
            iconSize="sm"
            active={editing}
            title={t("mcp.edit", { server: server.name })}
            aria-expanded={editing}
            aria-controls={panelId}
            onClick={() => setEditing((value) => !value)}
          />
        </div>
      </div>

      {editing && (
        <div id={panelId} {...stylex.props(ss.afterRow)}>
          <ServerForm
            server={server}
            onDone={() => setEditing(false)}
            onCancel={() => setEditing(false)}
          />
        </div>
      )}
    </div>
  );
}
