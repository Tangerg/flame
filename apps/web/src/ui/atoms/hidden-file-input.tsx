import * as stylex from "@stylexjs/stylex";
import { type FileInputPrimitiveProps } from "@/ui/primitives/index";
import { FileInputPrimitive } from "@/ui/primitives/index";
const styles = stylex.create({ away: { display: "none" } });

export type HiddenFileInputProps = Omit<FileInputPrimitiveProps, "className">;

export function HiddenFileInput(props: HiddenFileInputProps) {
  return <FileInputPrimitive {...props} {...stylex.props(styles.away)} />;
}
