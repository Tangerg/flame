import * as stylex from "@stylexjs/stylex";
import { type ComponentPropsWithoutRef } from "react";
import { RecordGroup } from "@flame/ui";
const styles = stylex.create({
  group: { width: "100%", containerType: "inline-size" },
});

export function SettingsGroup({
  styles: extra,
  children,
  ...props
}: Omit<ComponentPropsWithoutRef<typeof RecordGroup>, "variant">) {
  return (
    <RecordGroup {...props} variant="list" data-focus-inset="" styles={[styles.group, extra]}>
      {children}
    </RecordGroup>
  );
}
