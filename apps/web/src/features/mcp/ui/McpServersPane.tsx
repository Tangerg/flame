import * as stylex from "@stylexjs/stylex";
import { useId, useState } from "react";
import { DataView } from "@/ui/index";
import { Icon } from "@/ui/index";
import { Button, RecordGroup } from "@flame/ui";
import { useT } from "@/lib/i18n/index";
import { useMCPServers } from "@/features/mcp/application/mcpServerQueries";
import { mcpServerLabel } from "@/lib/toolSource";
import { useMCPServerCommands } from "@/features/mcp/application/mcpServerConfig";
import { JsonImport } from "@/features/mcp/ui/JsonImport";
import { ServerForm } from "@/features/mcp/ui/ServerForm";
import { ServerRow } from "@/features/mcp/ui/ServerRow";
import { settingStyles as ss } from "@/ui/settings/settingStyles";
export function McpServersPane() {
  const t = useT();
  const commands = useMCPServerCommands();
  const { data, isPending, error, refetch } = useMCPServers();
  const [adding, setAdding] = useState<HTMLElement | null>(null);
  const panelId = useId();

  return (
    <div {...stylex.props(ss.stack)}>
      <div {...stylex.props(ss.split)}>
        <JsonImport />
        <Button
          variant="primary"
          size="sm"
          disabled={adding !== null || commands === null}
          aria-expanded={adding !== null}
          aria-controls={panelId}
          onClick={(event) => setAdding(event.currentTarget)}
        >
          <Icon name="plus" size="sm" />
          {t("mcp.add")}
        </Button>
      </div>

      {adding && (
        <div id={panelId}>
          <ServerForm
            opener={adding}
            onDone={() => setAdding(null)}
            onCancel={() => setAdding(null)}
          />
        </div>
      )}

      <DataView
        items={data}
        isLoading={isPending}
        failure={error}
        onRetry={refetch}
        skeletonCount={3}
        empty={adding ? undefined : { icon: "server", title: t("mcp.empty") }}
      >
        {(rows) => (
          <RecordGroup variant="list">
            {rows.map((s) => (
              <ServerRow key={mcpServerLabel(s.id)} server={s} />
            ))}
          </RecordGroup>
        )}
      </DataView>
    </div>
  );
}
