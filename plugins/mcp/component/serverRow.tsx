import { useId, useState, useSyncExternalStore } from "react";
import type { ComponentContext, Translate } from "@flame/component-sdk";
import { Pencil } from "@flame/icons/react";
import {
  Badge,
  Button,
  Disclosure,
  IconButton,
  ListRow,
  RecordBody,
  RecordHeading,
  RecordMeta,
  Switch,
  type Tone,
} from "@flame/ui";
import { serverKey, type MCPServerSettings } from "./model";
import type { MCPSettings } from "./settings";
import { ServerForm } from "./serverForm";
import { ToolControls } from "./toolControls";
import { CommandFeedback } from "./feedback";
const TONES: Record<MCPServerSettings["status"], Tone> = {
  disabled: "neutral",
  connected: "success",
  connecting: "accent",
  needsAuth: "warning",
  failed: "negative",
  disconnected: "neutral",
};
interface ServerRowProps {
  context: ComponentContext;
  settings: MCPSettings;
  server: MCPServerSettings;
  t: Translate;
}

function ServerConnectionButton({
  context,
  settings,
  server,
  t,
  signingIn,
  running,
  busy,
}: ServerRowProps & { signingIn: boolean; running: boolean; busy: boolean }) {
  if (!server.enabled && !signingIn) return null;
  const needsSignIn = server.status === "needsAuth";
  const label = needsSignIn
    ? signingIn
      ? "mcp.signingIn"
      : "mcp.signIn"
    : "mcp.reconnect";
  return (
    <Button
      variant={needsSignIn ? "primary" : "outline"}
      size="sm"
      disabled={busy || signingIn}
      pending={running || server.status === "connecting" || signingIn}
      aria-label={t(`${label}.aria`, { server: server.id.name })}
      onClick={() =>
        void (needsSignIn
          ? settings.authorize(context, server.id)
          : settings.reconnect(context, server.id))
      }
    >
      {t(label)}
    </Button>
  );
}

export function ServerRow({ context, settings, server, t }: ServerRowProps) {
  const state = useSyncExternalStore(settings.subscribe, settings.getSnapshot);
  const key = serverKey(server.id);
  const command = state.commands.get(key);
  const busy = settings.busy(key);
  const [editing, setEditing] = useState<HTMLElement | null>(null);
  const panelId = useId();
  const authorization = state.authorizations.get(key);
  const signingIn = authorization?.status.type === "pending";
  return (
    <ListRow>
      <RecordHeading
        title={
          <span style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>
            {server.id.name}
          </span>
        }
        description={
          <RecordMeta>
            <Badge tone={TONES[server.status]}>
              {t(`mcp.status.${server.status}`)}
            </Badge>
            <Badge>
              {t(
                server.type === "stdio"
                  ? "mcp.transport.stdio"
                  : "mcp.transport.http",
              )}
            </Badge>
            {server.status === "connected" && (
              <span>
                {t("mcp.toolCount", { count: server.toolCount ?? 0 })}
              </span>
            )}
          </RecordMeta>
        }
        actions={
          <>
            <ServerConnectionButton
              context={context}
              settings={settings}
              server={server}
              t={t}
              signingIn={signingIn}
              running={command?.phase === "running"}
              busy={busy}
            />
            {server.id.origin.type === "user" && (
              <>
                <Switch
                  checked={server.enabled}
                  disabled={busy || signingIn}
                  onCheckedChange={(value) =>
                    void settings.setEnabled(context, server.id, value)
                  }
                  ariaLabel={t("mcp.enable.aria", { server: server.id.name })}
                />
                <IconButton
                  icon={Pencil}
                  size="sm"
                  active={editing !== null}
                  aria-label={t("mcp.edit", { server: server.id.name })}
                  aria-expanded={editing !== null}
                  aria-controls={panelId}
                  onClick={(event) =>
                    setEditing(editing ? null : event.currentTarget)
                  }
                />
              </>
            )}
          </>
        }
      />
      {server.status === "failed" && server.errorDetail && (
        <RecordBody>{t(`mcpStatus.${server.errorDetail}`)}</RecordBody>
      )}
      {authorization?.status.type === "unconfirmed" && (
        <div role="status">
          <RecordBody>{t("mcp.signIn.unconfirmed")}</RecordBody>
        </div>
      )}
      {authorization?.status.type === "failed" && (
        <div role="alert">
          <RecordBody>{t("mcpStatus.mcp_authorization_failed")}</RecordBody>
        </div>
      )}
      {signingIn && state.feedback.get(key)?.kind === "error" && (
        <Button onClick={() => settings.retryAuthorization(context, server.id)}>
          {t("retry")}
        </Button>
      )}
      {!editing && (
        <CommandFeedback
          context={context}
          settings={settings}
          resource={key}
          t={t}
        />
      )}
      <Disclosure title={t("mcp.tools.title", { server: server.id.name })}>
        <ToolControls
          context={context}
          settings={settings}
          server={server.id}
          t={t}
        />
      </Disclosure>
      {editing && (
        <div id={panelId}>
          <ServerForm
            context={context}
            settings={settings}
            t={t}
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
