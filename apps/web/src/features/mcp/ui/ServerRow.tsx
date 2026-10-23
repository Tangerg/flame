import { iconGlyph } from "@/ui/icons/index";
import { failureText } from "@/lib/rpcErrors";
import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useCommandAction } from "@/features/command/public/commandAction";
import { useEffect, useId, useRef, useState } from "react";
import { StatusDot } from "@/ui/index";
import { Switch } from "@flame/ui";
import { Tag } from "@/ui/index";
import {
  Button,
  Disclosure,
  IconButton,
  ListRow,
  RecordBody,
  RecordHeading,
  RecordMeta,
  vocab,
} from "@flame/ui";
import { type MCPServerSettings } from "@/features/mcp/application/mcpServerConfig";
import {
  type MCPTransport,
  reconnectMCPServer,
  useAuthorizeMCPServer,
  useSetMCPServerEnabled,
} from "@/features/mcp/application/mcpServerConfig";
import { notifyError } from "@/features/notifications/public/notifications";
import { type DotTone } from "@/lib/tone";
import { useT } from "@/lib/i18n/index";
import { ServerForm } from "@/features/mcp/ui/ServerForm";
import { ToolControls } from "@/features/mcp/ui/ToolControls";
import { face, layout, type as typeStep } from "@flame/ui/tokens.stylex";
const STATUS_TONE: Record<MCPServerSettings["status"], DotTone> = {
  disabled: "idle",
  connected: "ok",
  connecting: "running",
  needsAuth: "waiting",
  failed: "err",
  disconnected: "idle",
};

const STATUS_TEXT: Record<MCPServerSettings["status"], string> = {
  disabled: "mcp.status.disabled",
  connected: "mcp.status.connected",
  connecting: "mcp.status.connecting",
  needsAuth: "mcp.status.needsAuth",
  failed: "mcp.status.failed",
  disconnected: "mcp.status.disconnected",
};

function TransportBadge({ transport }: { transport: MCPTransport }) {
  const t = useT();
  return (
    <Tag size="sm">{t(transport === "stdio" ? "mcp.transport.stdio" : "mcp.transport.http")}</Tag>
  );
}

const sr = stylex.create({
  identity: {
    display: "grid",
    gridTemplateColumns: "auto minmax(0, 1fr)",
    alignItems: "start",
    gap: layout.contentGap,
    minWidth: 0,
  },
  name: { whiteSpace: "pre-wrap" },
});

export function ServerRow({ server }: { server: MCPServerSettings }) {
  const t = useT();
  const setEnabled = useSetMCPServerEnabled();
  const authorize = useAuthorizeMCPServer();
  const [editing, setEditing] = useState<HTMLElement | null>(null);
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

  const toggleAction = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("mcp.error.toggle"),
    source: "mcp",
  });
  const onToggle = (enabled: boolean) => toggleAction.run(() => setEnabled(server.id, enabled));

  const reconnectAction = useCommandAction({
    wasRetired: wasGenerationRetired,
    fallback: t("mcp.error.reconnect"),
    source: "mcp",
  });

  const onSignIn = async () => {
    const controller = new AbortController();
    authorizationController.current?.abort();
    authorizationController.current = controller;
    setSigningIn(true);
    try {
      await authorize(server.id, controller.signal);
    } catch (err) {
      if (controller.signal.aborted || wasGenerationRetired(err)) return;
      notifyError(failureText(err, t("mcp.error.signIn")), { source: "mcp" });
    } finally {
      if (authorizationController.current === controller) {
        authorizationController.current = null;
        setSigningIn(false);
      }
    }
  };

  const tone = STATUS_TONE[server.status];
  const active = server.status === "connected";
  const authorizing = server.status === "needsAuth" || signingIn;
  const connectionPending = reconnectAction.busy || server.status === "connecting" || signingIn;
  const onConnection = () => {
    if (authorizing) void onSignIn();
    else reconnectAction.run(() => reconnectMCPServer(server.id));
  };

  return (
    <ListRow>
      <RecordHeading
        title={
          <span {...stylex.props(sr.identity)}>
            <span {...stylex.props(vocab.firstLine)}>
              <StatusDot tone={tone} />
            </span>
            <span {...stylex.props(sr.name)}>{server.id.name}</span>
          </span>
        }
        description={
          <RecordMeta>
            <span>{t(STATUS_TEXT[server.status])}</span>
            <TransportBadge transport={server.type} />
            {active && (
              <span {...stylex.props(face.mono)}>
                {t("mcp.toolCount", { count: server.toolCount ?? 0 })}
              </span>
            )}
          </RecordMeta>
        }
        actions={
          <>
            {(server.enabled || signingIn) && (
              <Button
                variant={authorizing ? "primary" : "outline"}
                size="sm"
                pending={connectionPending}
                aria-label={t(
                  authorizing
                    ? signingIn
                      ? "mcp.signingIn.aria"
                      : "mcp.signIn.aria"
                    : "mcp.reconnect.aria",
                  { server: server.id.name },
                )}
                onClick={onConnection}
              >
                {t(authorizing ? (signingIn ? "mcp.signingIn" : "mcp.signIn") : "mcp.reconnect")}
              </Button>
            )}
            {server.id.origin.type === "user" && (
              <>
                <Switch
                  checked={server.enabled}
                  disabled={toggleAction.busy}
                  onCheckedChange={(value) => onToggle(value)}
                  ariaLabel={t("mcp.enable.aria", { server: server.id.name })}
                />
                <IconButton
                  icon={iconGlyph("edit")}
                  size="sm"
                  active={editing !== null}
                  aria-label={t("mcp.edit", { server: server.id.name })}
                  aria-expanded={editing !== null}
                  aria-controls={panelId}
                  onClick={(event) => setEditing(editing ? null : event.currentTarget)}
                />
              </>
            )}
          </>
        }
      />

      {server.status === "failed" && server.errorDetail && (
        <RecordBody>
          <span {...stylex.props(vocab.negative, typeStep.uiMd)}>{server.errorDetail}</span>
        </RecordBody>
      )}

      <Disclosure title={t("mcp.tools.title", { server: server.id.name })}>
        <ToolControls server={server.id} />
      </Disclosure>

      {editing && (
        <div id={panelId}>
          <ServerForm
            server={server}
            opener={editing}
            onDone={() => setEditing(null)}
            onCancel={() => setEditing(null)}
          />
        </div>
      )}
    </ListRow>
  );
}
