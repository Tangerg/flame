import { useRef } from "react";
import * as stylex from "@stylexjs/stylex";
import { color, leading, space, type, weight } from "@flame/ui/tokens.stylex";
import { CatalogList, type CatalogPickerGroup } from "@/ui/atoms/index";
const styles = stylex.create({
  root: {
    display: "flex",
    minHeight: 0,
    height: "100%",
    flexDirection: "column",
    padding: space.s4,
  },
  heading: { display: "flex", flexDirection: "column", gap: space.s1, paddingBottom: space.s4 },
  title: { margin: 0, fontWeight: weight.semibold, color: color.fg },
  description: { margin: 0, lineHeight: leading.body, color: color.fgMuted },
});

export function AgentDockCatalog({
  groups,
  title,
  description,
  placeholder,
  emptyLabel,
  onSelect,
}: {
  groups: CatalogPickerGroup[];
  title: string;
  description: string;
  placeholder: string;
  emptyLabel: string;
  onSelect: (id: string, origin: HTMLElement) => void;
}) {
  const searchRef = useRef<HTMLInputElement>(null);
  return (
    <div {...stylex.props(styles.root)}>
      <div {...stylex.props(styles.heading)}>
        <h2 {...stylex.props(styles.title, type.uiMd)}>{title}</h2>
        <p {...stylex.props(styles.description, type.uiSm)}>{description}</p>
      </div>
      <CatalogList
        groups={groups}
        label={title}
        placeholder={placeholder}
        emptyLabel={emptyLabel}
        inputRef={searchRef}
        onSelect={(item) => {
          if (searchRef.current) onSelect(item.id, searchRef.current);
        }}
      />
    </div>
  );
}
