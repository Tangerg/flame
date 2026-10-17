import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { DataView, Icon } from "@/ui";
import { Button, RecordGroup, vocab } from "@flame/ui";
import { useT } from "@/lib/i18n";
import { useMCPServers } from "../application/mcpServerQueries";
import { mcpServerLabel } from "@/lib/toolSource";
import { JsonImport } from "./JsonImport";
import { ServerForm } from "./ServerForm";
import { ServerRow } from "./ServerRow";
import { settingStyles as ss } from "../../kit/settingStyles";

const mp = stylex.create({ splitTop: { alignItems: "flex-start" } });

export function McpServersPane() {
  const t = useT();
  const { data, isLoading, error, refetch } = useMCPServers();
  const [adding, setAdding] = useState(false);

  return (
    <div {...stylex.props(ss.stack)}>
      <div {...stylex.props(ss.split, adding && mp.splitTop)}>
        {adding ? (
          <div {...stylex.props(vocab.grow)}>
            <ServerForm onDone={() => setAdding(false)} onCancel={() => setAdding(false)} />
          </div>
        ) : (
          <>
            <JsonImport />
            <Button variant="primary" size="sm" onClick={() => setAdding(true)}>
              <Icon name="plus" size="sm" />
              {t("mcp.add")}
            </Button>
          </>
        )}
      </div>

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
