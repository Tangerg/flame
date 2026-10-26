import { type ComponentPropsWithRef } from "react";
import * as stylex from "@stylexjs/stylex";
const styles = stylex.create({ away: { display: "none" } });

export type HiddenFileInputProps = Omit<ComponentPropsWithRef<"input">, "className" | "type">;

export function HiddenFileInput(props: HiddenFileInputProps) {
  return <input {...props} type="file" {...stylex.props(styles.away)} />;
}
