import { useRef } from "react";
import * as stylex from "@stylexjs/stylex";
import { layout, space } from "@flame/ui/tokens.stylex";
import { CatalogList, type CatalogPickerGroup } from "@/ui/atoms/index";
const styles = stylex.create({
  root: {
    display: "flex",
    minHeight: 0,
    height: "100%",
    flexDirection: "column",
    // Dock views sit inside a reserved focus edge; the catalog adds it so its text
    // starts on the same line as theirs and the tab labels', and its search field
    // sits where a panel's controls row puts its own.
    paddingInline: `calc(${layout.compactGutter} + var(--focus-inset-edge))`,
    paddingTop: `calc(${layout.compactGutter} + var(--focus-inset-edge) + ${layout.controlGap})`,
    paddingBottom: space.s4,
  },
  // Search and option fills bleed by the rows' inset; their text keeps the column edge.
  catalog: { display: "flex", flex: 1, minHeight: 0, marginInline: `calc(-1 * ${space.s2})` },
});

export function AgentDockCatalog({
  groups,
  title,
  placeholder,
  emptyLabel,
  clearLabel,
  onSelect,
}: {
  groups: CatalogPickerGroup[];
  title: string;
  placeholder: string;
  emptyLabel: string;
  clearLabel: string;
  onSelect: (id: string, origin: HTMLElement) => void;
}) {
  const searchRef = useRef<HTMLInputElement>(null);
  return (
    <div {...stylex.props(styles.root)}>
      <h2 className="sr-only">{title}</h2>
      <div {...stylex.props(styles.catalog)}>
        <CatalogList
          groups={groups}
          label={title}
          placeholder={placeholder}
          emptyLabel={emptyLabel}
          clearLabel={clearLabel}
          inputRef={searchRef}
          onSelect={(item) => {
            if (searchRef.current) onSelect(item.id, searchRef.current);
          }}
        />
      </div>
    </div>
  );
}
