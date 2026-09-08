import * as stylex from "@stylexjs/stylex";
import { useState } from "react";
import { DataView, gap, Icon, PillButton, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { useMCPServers } from "../application/mcpServerQueries";
import { JsonImport } from "./JsonImport";
import { ServerForm } from "./ServerForm";
import { ServerRow } from "./ServerRow";
import { settingStyles as ss } from "../../kit/settingStyles";

const mp = stylex.create({ splitTop: { alignItems: "flex-start" } });

export function McpServersPane() {
  const t = useT();
  const { data, isLoading, isError, refetch } = useMCPServers();
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
            <PillButton variant="outlined" size="sm" onClick={() => setAdding(true)}>
              <Icon name="plus" size="sm" />
              {t("mcp.add")}
            </PillButton>
          </>
        )}
      </div>

      <DataView
        items={data}
        isLoading={isLoading}
        isError={isError}
        onRetry={refetch}
        skeletonCount={3}
        empty={{
          icon: "tool",
          title: t("mcp.empty"),
          sub: t("mcp.empty.sub"),
        }}
      >
        {(rows) => (
          <div {...stylex.props(vocab.column, gap.s2)}>
            {rows.map((s) => (
              <ServerRow key={s.name} server={s} />
            ))}
          </div>
        )}
      </DataView>
    </div>
  );
}
