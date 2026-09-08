import * as stylex from "@stylexjs/stylex";
import { wasGenerationRetired } from "@/lib/asyncOwnership";
import { MCP_SERVERS_PANE } from "@/plugins/builtin/settings/kit/panes";
import { useId, useRef, useState } from "react";
import { Badge, Icon, IconButton, Pressable, Tag, TextButton, knownIconName } from "@/ui";
import type { Tone } from "@/lib/tone";
import { useT } from "@/lib/i18n";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError } from "@/plugins/sdk";
import { openWorkspaceSettingsPane } from "@/plugins/builtin/workspace/public/navigation";
import { cn } from "@/lib/classNames";
import {
  type MCPServerSettings,
  reconnectMCPServer,
} from "@/plugins/builtin/settings/mcp-servers/public/serverCatalog";
import { useMCPServerToolConfigs } from "@/plugins/builtin/workspace/application/toolCatalog";
import { color, radius, space, surface, type as typeStep } from "@/styles/tokens.stylex";
import { viewStyles as vs } from "./viewStyles";

// The status is this view's business; how a tone is painted is the Badge's. Before, this
// table carried its own palette (`-wash` fills, coloured ink) beside the one every other
// badge in the app uses, so two servers in two views reported the same state in two skins.
const STATUS_BADGE: Record<MCPServerSettings["status"], { key: string; tone: Tone }> = {
  disabled: { key: "tools.status.off", tone: "neutral" },
  connecting: { key: "tools.status.connecting", tone: "neutral" },
  connected: { key: "tools.status.on", tone: "accent" },
  disconnected: { key: "tools.status.off", tone: "neutral" },
  failed: { key: "tools.status.error", tone: "negative" },
  needsAuth: { key: "tools.status.login", tone: "warning" },
};

// The tool list hangs under the server's NAME, past the 40px plate and its gap, so a tool
// reads as belonging to the row above rather than starting a column of its own.
const TOOL_INSET = "68px";

const mr = stylex.create({
  // The row publishes what its plate should look like, because the plate brightens when the
  // POINTER IS ON THE ROW rather than on the plate — `group-hover/` with no ancestor selector.
  row: {
    "--plate-fill": { default: surface.surface2, ":hover": surface.surface3 },
    "--plate-ink": { default: color.fgMuted, ":hover": color.fg },
    display: "grid",
    gridTemplateColumns: "calc(var(--spacing) * 10) 1fr auto auto auto",
    alignItems: "center",
    gap: space.s3,
    paddingBlock: space.s3,
  },
  plate: {
    display: "grid",
    height: space.s10,
    width: space.s10,
    placeItems: "center",
    borderRadius: radius.lg,
    backgroundColor: "var(--plate-fill)",
    color: "var(--plate-ink)",
    transitionProperty: "background-color, color",
  },
  name: {
    minWidth: 0,
    borderWidth: 0,
    backgroundColor: "transparent",
    padding: 0,
    textAlign: "left",
  },
  desc: { marginTop: space.s0_5, color: color.fgFaint },
  toolNote: {
    margin: 0,
    paddingInline: "var(--density-column-gutter-wide)",
    paddingBottom: space.s3,
    paddingLeft: TOOL_INSET,
  },
  toolList: {
    margin: 0,
    listStyle: "none",
    paddingInline: "var(--density-column-gutter-wide)",
    paddingBottom: space.s3,
    paddingLeft: TOOL_INSET,
  },
  toolItem: { paddingBlock: space.s0_5 },
  toolFoot: {
    paddingInline: "var(--density-column-gutter-wide)",
    paddingBottom: space.s3,
    paddingLeft: TOOL_INSET,
  },
});

function McpToolList({ server }: { server: string }) {
  const t = useT();
  const { data: tools, isLoading } = useMCPServerToolConfigs(server);
  if (isLoading)
    return (
      <p {...stylex.props(mr.toolNote, vs.caption, typeStep.uiSm)}>{t("tools.loadingTools")}</p>
    );
  if (!tools?.length)
    return <p {...stylex.props(mr.toolNote, vs.caption, typeStep.uiSm)}>{t("tools.noTools")}</p>;
  return (
    <ul {...stylex.props(mr.toolList)}>
      {tools.map((tool) => (
        <li key={tool.name} {...stylex.props(vs.entryPlain, mr.toolItem)}>
          <Tag size="sm" ink="strong">
            {tool.name}
          </Tag>
          <span {...stylex.props(vs.truncate, vs.caption, typeStep.uiSm)} title={tool.description}>
            {tool.description}
          </span>
        </li>
      ))}
    </ul>
  );
}

function McpAuthGuide({ server }: { server: string }) {
  const t = useT();
  const openConfig = () => {
    openWorkspaceSettingsPane(MCP_SERVERS_PANE);
  };
  return (
    <div {...stylex.props(vs.line, mr.toolFoot)}>
      <TextButton onClick={openConfig}>
        <Icon name="settings" size="sm" />
        {t("tools.auth.configure", { server })}
      </TextButton>
    </div>
  );
}

export function McpRow({ server }: { server: MCPServerSettings }) {
  const t = useT();
  const status = STATUS_BADGE[server.status];
  const reconnectingRef = useRef(false);
  const [reconnecting, setReconnecting] = useState(false);
  const connecting = reconnecting || server.status === "connecting";
  const [open, setOpen] = useState(false);
  const panelId = useId();

  const reconnect = async (): Promise<void> => {
    if (connecting || reconnectingRef.current || server.status === "disabled") return;
    reconnectingRef.current = true;
    setReconnecting(true);
    try {
      await reconnectMCPServer(server.id);
    } catch (cause) {
      if (!wasGenerationRetired(cause)) {
        notifyError(rpcErrorText(cause) ?? t("tools.reconnectFailed", { server: server.id }));
      }
    } finally {
      reconnectingRef.current = false;
      setReconnecting(false);
    }
  };

  return (
    <div>
      <div {...stylex.props(mr.row, vs.gutter, vs.wash)}>
        <div {...stylex.props(mr.plate)}>
          <Icon name={knownIconName(server.icon) ?? "tool"} size="md" />
        </div>
        <Pressable
          type="button"
          aria-expanded={open}
          aria-controls={panelId}
          onClick={() => setOpen((v) => !v)}
          className={stylex.props(mr.name).className}
        >
          <div {...stylex.props(vs.title, vs.truncate, typeStep.uiMd)}>{server.name}</div>
          <div {...stylex.props(mr.desc, vs.truncate, typeStep.uiMd)}>{server.desc}</div>
        </Pressable>
        <Badge size="md">{t("mcp.toolCount", { count: server.tools })}</Badge>
        <Badge
          size="md"
          tone={status.tone}
          className={cn(server.status === "connecting" && "animate-pulse")}
          title={server.status === "failed" ? server.errorDetail : undefined}
        >
          {t(status.key)}
        </Badge>
        <IconButton
          icon="loop"
          iconSize="sm"
          title={t("tools.reconnect")}
          disabled={connecting || server.status === "disabled"}
          onClick={() => void reconnect()}
          className={cn(connecting && "animate-spin")}
        />
      </div>
      {open && (
        <div id={panelId}>
          {server.status === "needsAuth" ? (
            <McpAuthGuide server={server.id} />
          ) : (
            <McpToolList server={server.id} />
          )}
        </div>
      )}
    </div>
  );
}
