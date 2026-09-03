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

function McpToolList({ server }: { server: string }) {
  const t = useT();
  const { data: tools, isLoading } = useMCPServerToolConfigs(server);
  if (isLoading)
    return (
      <p className="m-0 px-4 pb-3 pl-[68px] text-ui-sm text-fg-faint">{t("tools.loadingTools")}</p>
    );
  if (!tools?.length)
    return <p className="m-0 px-4 pb-3 pl-[68px] text-ui-sm text-fg-faint">{t("tools.noTools")}</p>;
  return (
    <ul className="m-0 list-none px-4 pb-3 pl-[68px]">
      {tools.map((tool) => (
        <li key={tool.name} className="flex items-baseline gap-2 py-0.5">
          <Tag size="sm" ink="strong">
            {tool.name}
          </Tag>
          <span className="truncate text-ui-sm text-fg-faint" title={tool.description}>
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
    <div className="flex items-center gap-2 px-4 pb-3 pl-[68px]">
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
      <div className="group grid grid-cols-[40px_1fr_auto_auto_auto] items-center gap-3 px-4 py-3 hover:bg-hover transition-colors">
        <div className="grid h-10 w-10 place-items-center rounded-lg bg-surface-2 text-fg-muted group-hover:bg-surface-3 group-hover:text-fg transition-colors">
          <Icon name={knownIconName(server.icon) ?? "tool"} size="md" />
        </div>
        <Pressable
          type="button"
          aria-expanded={open}
          aria-controls={panelId}
          onClick={() => setOpen((v) => !v)}
          className="min-w-0 border-0 bg-transparent p-0 text-left"
        >
          <div className="text-ui-md font-semibold text-fg truncate">{server.name}</div>
          <div className="mt-0.5 text-ui-md text-fg-faint truncate">{server.desc}</div>
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
