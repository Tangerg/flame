import { type ComponentPropsWithRef } from "react";
import * as stylex from "@stylexjs/stylex";
export type ColorPickerInputProps = Omit<ComponentPropsWithRef<"input">, "className" | "type">;

const styles = stylex.create({
  overlay: {
    position: "absolute",
    inset: 0,
    height: "100%",
    width: "100%",
    cursor: "pointer",
    opacity: 0,
  },
});

export function ColorPickerInput(props: ColorPickerInputProps) {
  return <input {...props} type="color" {...stylex.props(styles.overlay)} />;
}
