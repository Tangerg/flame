import type { CatalogPickerGroup } from "@/ui/atoms";
import { AgentRow } from "./navigation-row";

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
    <div className="flex min-h-0 flex-1 flex-col overflow-y-auto px-2 pt-2 pb-4">
      <div className="px-2 pb-1 text-ui-xs font-medium text-fg-faint">{title}</div>
      {groups.map((group) => (
        <section key={group.id} className="pt-2">
          <div className="px-2 pb-1 text-ui-2xs font-medium tracking-normal text-fg-faint">
            {group.label}
          </div>
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
