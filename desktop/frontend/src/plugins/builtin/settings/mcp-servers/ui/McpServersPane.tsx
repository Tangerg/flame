import * as stylex from "@stylexjs/stylex";
import { useId, useState } from "react";
import { DataView, Icon } from "@/ui";
import { Button, RecordGroup } from "@flame/ui";
import { useT } from "@/lib/i18n";
import { useMCPServers } from "../application/mcpServerQueries";
import { mcpServerLabel } from "@/lib/toolSource";
import { JsonImport } from "./JsonImport";
import { ServerForm } from "./ServerForm";
import { ServerRow } from "./ServerRow";
import { settingStyles as ss } from "../../kit/settingStyles";

export function McpServersPane() {
  const t = useT();
  const { data, isLoading, error, refetch } = useMCPServers();
  const [adding, setAdding] = useState<HTMLElement | null>(null);
  const panelId = useId();

  return (
    <div {...stylex.props(ss.stack)}>
      <div {...stylex.props(ss.split)}>
        <JsonImport />
        <Button
          variant="primary"
          size="sm"
          disabled={adding !== null}
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
        isLoading={isLoading}
        failure={error}
        onRetry={refetch}
        skeletonCount={3}
        empty={{
          icon: "server",
          title: t("mcp.empty"),
          sub: t("mcp.empty.sub"),
        }}
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
