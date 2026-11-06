import { useState } from "react";
import * as stylex from "@stylexjs/stylex";
import { AgentRow, AgentStatusPill } from "@/ui/agent/index";
import { businessMessages as messages } from "./businessMessages";
import { businessStyles as styles } from "./businessStyles";
import {
  businessVariant,
  navigationVariants,
  statusTones,
  statusVariants,
} from "./businessVariants";
import { localize, type LabExampleProps } from "./story";

const NAVIGATION_ROWS = [
  { id: "overview", label: messages.samples.overview, icon: "folder" },
  { id: "tasks", label: messages.samples.tasks, icon: "list" },
  { id: "history", label: messages.samples.history, icon: "chat" },
] as const;

export function StatusExample({ variant, locale }: LabExampleProps) {
  const selected = businessVariant(statusVariants, variant);
  return (
    <div {...stylex.props(styles.row)}>
      {selected.longLabel ? (
        <AgentStatusPill tone="waiting">
          {localize(messages.samples.longStatus, locale)}
        </AgentStatusPill>
      ) : (
        statusTones.map((tone) => (
          <AgentStatusPill key={tone} tone={tone}>
            {localize(messages.samples.status[tone], locale)}
          </AgentStatusPill>
        ))
      )}
    </div>
  );
}

export function NavigationExample({ variant, locale, observe }: LabExampleProps) {
  const scenario = businessVariant(navigationVariants, variant);
  const [selected, setSelected] = useState<string>(NAVIGATION_ROWS[0].id);
  return (
    <div {...stylex.props(styles.stack)}>
      <div
        role="group"
        aria-label={localize(messages.stories.navigation.title, locale)}
        {...stylex.props(styles.rows)}
      >
        {NAVIGATION_ROWS.map((row) => (
          <AgentRow
            key={row.id}
            icon={row.icon}
            look={scenario.look}
            active={selected === row.id}
            disabled={scenario.disabled}
            detail={scenario.detail ? localize(messages.samples.rowDetail, locale) : undefined}
            revealOverflow={scenario.longLabel}
            aria-pressed={selected === row.id}
            onClick={() => {
              setSelected(row.id);
              observe("navigation.select", row.id);
            }}
          >
            {scenario.longLabel
              ? `${localize(row.label, locale)} · ${localize(messages.samples.longRow, locale)}`
              : localize(row.label, locale)}
          </AgentRow>
        ))}
      </div>
      <output
        aria-label={localize(messages.samples.selected, locale)}
        {...stylex.props(styles.caption)}
      >
        {selected}
      </output>
    </div>
  );
}
