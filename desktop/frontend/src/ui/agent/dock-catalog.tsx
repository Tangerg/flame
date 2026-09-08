import * as stylex from "@stylexjs/stylex";
import { color, space, type, weight } from "@/styles/tokens.stylex";
import type { CatalogPickerGroup } from "@/ui/atoms";
import { AgentRow } from "./navigation-row";

const styles = stylex.create({
  port: {
    display: "flex",
    minHeight: 0,
    flex: 1,
    flexDirection: "column",
    overflowY: "auto",
    paddingInline: space.s2,
    paddingTop: space.s2,
    paddingBottom: space.s4,
  },
  title: {
    paddingInline: space.s2,
    paddingBottom: space.s1,
    fontWeight: weight.medium,
    color: color.fgFaint,
  },
  group: { paddingTop: space.s2 },
  // A group's own heading sits a step below the catalogue's, and drops the UI tracking that
  // crowds a word set in small caps.
  groupLabel: {
    paddingInline: space.s2,
    paddingBottom: space.s1,
    fontWeight: weight.medium,
    letterSpacing: "var(--tracking-none)",
    color: color.fgFaint,
  },
});

export function AgentDockCatalog({
  groups,
  title,
  onSelect,
}: {
  groups: readonly CatalogPickerGroup[];
  title: string;
  onSelect: (id: string) => void;
}) {
  return (
    <div {...stylex.props(styles.port)}>
      <div {...stylex.props(styles.title, type.uiXs)}>{title}</div>
      {groups.map((group) => (
        <section key={group.id} {...stylex.props(styles.group)}>
          <div {...stylex.props(styles.groupLabel, type.ui2xs)}>{group.label}</div>
          {group.items.map((item) => (
            <AgentRow
              key={item.id}
              icon={item.icon}
              detail={typeof item.description === "string" ? item.description : undefined}
              active={item.active}
              onClick={() => onSelect(item.id)}
            >
              {item.label}
            </AgentRow>
          ))}
        </section>
      ))}
    </div>
  );
}
