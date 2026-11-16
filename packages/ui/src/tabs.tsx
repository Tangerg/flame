import { Tabs as BaseTabs } from "@base-ui/react/tabs";
import * as stylex from "@stylexjs/stylex";
import { clsx } from "clsx";
import type { ReactNode } from "react";
import { dress } from "./button";
import { ContentVisibility } from "./content-visibility";
import { space, type as typeStep } from "./tokens.stylex";

const styles = stylex.create({
  root: {
    display: "flex",
    flexDirection: "column",
    gap: space.s3,
    minWidth: 0,
  },
  list: { display: "flex", flexWrap: "wrap", gap: space.s1 },
  panel: { minWidth: 0 },
});

export function Tabs<T extends string | number>({
  value,
  onValueChange,
  ariaLabel,
  items,
  className,
}: {
  value: T;
  onValueChange(value: T): void;
  ariaLabel: string;
  items: readonly {
    value: T;
    label: string;
    content: ReactNode;
    disabled?: boolean;
  }[];
  className?: string;
}) {
  const root = stylex.props(styles.root);
  return (
    <BaseTabs.Root
      value={value}
      orientation="horizontal"
      onValueChange={(next) => {
        const item = items.find((item) => item.value === next);
        if (item) onValueChange(item.value);
      }}
      {...root}
      className={clsx(root.className, className)}
    >
      <BaseTabs.List
        aria-label={ariaLabel}
        activateOnFocus
        {...stylex.props(styles.list)}
      >
        {items.map((item) => (
          <BaseTabs.Tab
            key={item.value}
            value={item.value}
            disabled={item.disabled}
            {...stylex.props(
              dress({ variant: "soft", active: item.value === value }),
              typeStep.uiMd,
            )}
          >
            {item.label}
          </BaseTabs.Tab>
        ))}
      </BaseTabs.List>
      {items.map((item) => (
        <BaseTabs.Panel
          key={item.value}
          value={item.value}
          keepMounted
          {...stylex.props(styles.panel)}
        >
          <ContentVisibility visible={value === item.value}>
            {item.content}
          </ContentVisibility>
        </BaseTabs.Panel>
      ))}
    </BaseTabs.Root>
  );
}
