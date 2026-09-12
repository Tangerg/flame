import * as stylex from "@stylexjs/stylex";
import { DataView, Switch, vocab } from "@/ui";
import { useT } from "@/lib/i18n";
import { useMCPTools } from "../application/mcpServerQueries";
import { color, radius, space, surface, type as typeStep, weight } from "@/styles/tokens.stylex";
import { settingStyles as ss } from "../../kit/settingStyles";

interface Props {
  server: string;
  disabledTools: string[];
  autoApproveTools: string[];
  onChange: (next: { disabledTools: string[]; autoApproveTools: string[] }) => void;
}

const tc = stylex.create({
  panel: { borderRadius: radius.card, backgroundColor: surface.sunken, padding: space.s2_5 },
  grid: {
    display: "grid",
    gridTemplateColumns: "minmax(0, 1fr) auto auto",
    alignItems: "center",
    columnGap: space.s4,
  },
  head: {
    rowGap: space.s1,
    paddingInline: space.s1_5,
    paddingBottom: space.s1_5,
    color: color.fgMuted,
    fontWeight: weight.medium,
  },
  row: {
    borderRadius: radius.card,
    backgroundColor: { default: null, ":hover": surface.hover },
    paddingInline: space.s1_5,
    paddingBlock: space.s1_5,
    transitionProperty: "background-color",
  },
  switchCol: { width: space.s12, textAlign: "center" },
  switchCell: { display: "flex", width: space.s12, justifyContent: "center" },
});

export function ToolControls({ server, disabledTools, autoApproveTools, onChange }: Props) {
  const t = useT();
  const { data, isLoading, isError, refetch } = useMCPTools({ server });

  const disabled = new Set(disabledTools);
  const autoApprove = new Set(autoApproveTools);

  const setDisabled = (name: string, isDisabled: boolean) => {
    const d = new Set(disabled);
    const a = new Set(autoApprove);
    if (isDisabled) {
      d.add(name);
      a.delete(name);
    } else {
      d.delete(name);
    }
    onChange({ disabledTools: [...d], autoApproveTools: [...a] });
  };

  const setAutoApprove = (name: string, on: boolean) => {
    const a = new Set(autoApprove);
    if (on) {
      a.add(name);
    } else {
      a.delete(name);
    }
    onChange({ disabledTools: [...disabled], autoApproveTools: [...a] });
  };

  return (
    <div {...stylex.props(tc.panel)}>
      <div {...stylex.props(tc.grid, tc.head, typeStep.uiSm)}>
        <span>{t("mcp.tools.tool")}</span>
        <span {...stylex.props(tc.switchCol)}>{t("mcp.tools.enabled")}</span>
        <span {...stylex.props(tc.switchCol)}>{t("mcp.tools.autoApprove")}</span>
      </div>
      <DataView
        items={data}
        isLoading={isLoading}
        isError={isError}
        onRetry={refetch}
        skeletonCount={3}
        empty={{ icon: "tool", title: t("mcp.tools.empty") }}
      >
        {(tools) => (
          <div {...stylex.props(vocab.column)}>
            {tools.map((tool) => {
              const isDisabled = disabled.has(tool.name);
              return (
                <div key={tool.name} {...stylex.props(tc.grid, tc.row)}>
                  <code
                    {...stylex.props(ss.monoName, typeStep.uiMd)}
                    title={tool.description || tool.name}
                  >
                    {tool.name}
                  </code>
                  <div {...stylex.props(tc.switchCell)}>
                    <Switch
                      checked={!isDisabled}
                      onCheckedChange={(on) => setDisabled(tool.name, !on)}
                      ariaLabel={t("mcp.tools.enable.aria", { tool: tool.name })}
                    />
                  </div>
                  <div {...stylex.props(tc.switchCell)}>
                    <Switch
                      checked={autoApprove.has(tool.name)}
                      disabled={isDisabled}
                      onCheckedChange={(on) => setAutoApprove(tool.name, on)}
                      ariaLabel={t("mcp.tools.autoApprove.aria", { tool: tool.name })}
                    />
                  </div>
                </div>
              );
            })}
          </div>
        )}
      </DataView>
    </div>
  );
}
