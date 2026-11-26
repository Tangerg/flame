import * as token from "@flame/ui/tokens.stylex";
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
  RecordSummary,
  Switch,
  type Tone,
} from "@flame/ui";
import { serverKey, type MCPServerSettings } from "./model";
import { shouldObserveAuthorization, type MCPSettings } from "./settings";
import { ServerForm } from "./serverForm";
import { ToolControls } from "./toolControls";
import { CommandFeedback } from "./feedback";
// Connected is the resting state; only the others are named.
const TONES: Record<Exclude<MCPServerSettings["status"], "connected">, Tone> = {
  disabled: "neutral",
  connecting: "accent",
  needsAuth: "warning",
  failed: "negative",
  disconnected: "neutral",
};
// Messages and the editor sit below the row on its text edge; empty, the column takes no space.
const below = {
  display: "grid",
  gap: token.layout.contentGap,
  minWidth: 0,
  paddingInline: token.rowInset.inline,
} as const;
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
  const needsSignIn = signingIn || server.status === "needsAuth";
  const label = needsSignIn
    ? signingIn
      ? "mcp.signingIn"
      : "mcp.signIn"
    : "mcp.reconnect";
  return (
    <Button
      variant={needsSignIn ? "primary" : "soft"}
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
  const observation = state.authorizations.get(key);
  const authorization = observation?.attempt;
  const signingIn =
    observation !== undefined && shouldObserveAuthorization(observation);
  const tools =
    server.toolCount === undefined
      ? undefined
      : t("mcp.toolCount", { count: server.toolCount });
  const name = (
    <span style={{ whiteSpace: "pre-wrap", overflowWrap: "anywhere" }}>
      {server.id.name}
    </span>
  );
  const status = server.status !== "connected" && (
    <Badge tone={TONES[server.status]}>
      {t(`mcp.status.${server.status}`)}
    </Badge>
  );
  const actions = (
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
  );
  return (
    <div>
      {server.toolCount === 0 ? (
        <ListRow>
          <RecordHeading
            title={name}
            status={status}
            description={tools}
            actions={actions}
          />
        </ListRow>
      ) : (
        <Disclosure
          variant="row"
          title={
            <RecordSummary title={name} status={status} description={tools} />
          }
          actions={actions}
        >
          <ToolControls
            context={context}
            settings={settings}
            server={server.id}
            t={t}
          />
        </Disclosure>
      )}
      <div style={below}>
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
          <Button
            onClick={() => settings.retryAuthorization(context, server.id)}
          >
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
        {editing && (
          <div id={panelId}>
            <ServerForm
              returnFocus={editing}
              context={context}
              settings={settings}
              t={t}
              server={server}
              onDone={() => setEditing(null)}
              onCancel={() => setEditing(null)}
            />
          </div>
        )}
      </div>
    </div>
  );
}
