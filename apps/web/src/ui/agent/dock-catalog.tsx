import { useRef } from "react";
import * as stylex from "@stylexjs/stylex";
import { color, space, type, weight } from "@flame/ui/tokens.stylex";
import { CatalogList, type CatalogPickerGroup } from "@/ui/atoms/index";
import { cn } from "@/lib/classNames";
const styles = stylex.create({
  root: {
    display: "flex",
    minHeight: 0,
    height: "100%",
    flexDirection: "column",
    padding: space.s4,
  },
  title: {
    marginTop: 0,
    marginInline: 0,
    marginBottom: space.s4,
    flexShrink: 0,
    fontWeight: weight.semibold,
    color: color.fg,
  },
});

export function AgentDockCatalog({
  groups,
  title,
  titleHidden = false,
  placeholder,
  emptyLabel,
  clearLabel,
  onSelect,
}: {
  groups: CatalogPickerGroup[];
  title: string;
  titleHidden?: boolean;
  placeholder: string;
  emptyLabel: string;
  clearLabel: string;
  onSelect: (id: string, origin: HTMLElement) => void;
}) {
  const searchRef = useRef<HTMLInputElement>(null);
  const heading = stylex.props(styles.title, type.uiMd);
  return (
    <div {...stylex.props(styles.root)}>
      <h2 {...heading} className={cn(heading.className, titleHidden && "sr-only")}>
        {title}
      </h2>
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
  );
}
