import { iconGlyph } from "@/ui/icons/index";
import { failureText } from "@/lib/rpcErrors";
import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { useEffect, useId, useRef, useState } from "react";
import { StatusDot } from "@/ui/index";
import { Switch } from "@flame/ui";
import { Tag } from "@/ui/index";
import {
  Button,
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
  useAuthorizeMCPServer,
  useSetMCPServerEnabled,
} from "@/features/mcp/application/mcpServerConfig";
import { notifyError } from "@/features/notifications/public/notifications";
import { type DotTone } from "@/lib/tone";
import { useT } from "@/lib/i18n/index";
import { ServerForm } from "@/features/mcp/ui/ServerForm";
import { face, layout, type as typeStep } from "@flame/ui/tokens.stylex";
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
  identity: {
    display: "grid",
    gridTemplateColumns: "auto minmax(0, 1fr)",
    alignItems: "start",
    gap: layout.controlGap,
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

  const onToggle = async (enabled: boolean) => {
    try {
      await setEnabled(server.id, enabled);
    } catch (err) {
      if (wasGenerationRetired(err)) return;
      notifyError(failureText(err, t("mcp.error.toggle")), { source: "mcp" });
    }
  };

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
            {(server.status === "needsAuth" || signingIn) && (
              <Button
                variant="primary"
                size="sm"
                pending={signingIn}
                onClick={() => void onSignIn()}
              >
                {t(signingIn ? "mcp.signingIn" : "mcp.signIn")}
              </Button>
            )}
            {server.id.origin.type === "user" && (
              <>
                <Switch
                  checked={server.enabled}
                  onCheckedChange={(value) => void onToggle(value)}
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
