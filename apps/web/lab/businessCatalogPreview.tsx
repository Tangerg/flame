import { useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { AgentDockCatalog } from "@/ui/agent/index";
import type { CatalogPickerGroup } from "@/ui/atoms/catalog-picker";
import { businessMessages as messages } from "./businessMessages";
import { businessStyles as styles } from "./businessStyles";
import { businessVariant, catalogVariants } from "./businessVariants";
import { localize, type LabExampleProps } from "./story";

const SAMPLE_PANELS = [
  { id: "files", label: messages.samples.files, icon: "folder" },
  { id: "changes", label: messages.samples.changes, icon: "file-diff" },
  { id: "trajectory", label: messages.samples.trajectory, icon: "workflow" },
  { id: "diagnostics", label: messages.samples.diagnostics, icon: "activity" },
] as const;

export function CatalogExample({ variant, locale, observe }: LabExampleProps) {
  const scenario = businessVariant(catalogVariants, variant);
  const [selected, setSelected] = useState<string | null>(null);
  const groups: CatalogPickerGroup[] = scenario.empty
    ? []
    : [
        {
          id: "workspace-materials",
          label: localize(messages.groups.workspace, locale),
          items: SAMPLE_PANELS.map((panel) => ({
            id: panel.id,
            label: localize(panel.label, locale),
            icon: panel.icon,
            active: selected === panel.id,
          })),
        },
      ];
  return (
    <div {...stylex.props(styles.stack)}>
      <div {...stylex.props(styles.catalog)}>
        <AgentDockCatalog
          groups={groups}
          title={localize(messages.samples.catalog, locale)}
          placeholder={localize(messages.samples.search, locale)}
          emptyLabel={localize(messages.samples.noMatches, locale)}
          clearLabel={localize(messages.samples.clear, locale)}
          onSelect={(id) => {
            setSelected(id);
            observe("catalog.select", id);
          }}
        />
      </div>
      {selected !== null && (
        <output
          aria-label={localize(messages.samples.selected, locale)}
          {...stylex.props(styles.caption)}
        >
          {selected}
        </output>
      )}
    </div>
  );
}
