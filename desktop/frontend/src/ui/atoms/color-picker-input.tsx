import * as stylex from "@stylexjs/stylex";
import type { ColorInputPrimitiveProps } from "@/ui/primitives";
import { ColorInputPrimitive } from "@/ui/primitives";

export type ColorPickerInputProps = Omit<ColorInputPrimitiveProps, "className">;

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
  return <ColorInputPrimitive {...props} {...stylex.props(styles.overlay)} />;
}
