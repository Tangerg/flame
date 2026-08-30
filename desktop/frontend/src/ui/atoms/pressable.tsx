import type { ReactNode } from "react";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@/ui/primitives";

export type PressableProps = Omit<ButtonPrimitiveProps, "children"> & {
  children?: ReactNode;
};

// For composite surfaces whose content owns its own layout (rows, cards, swatches,
// disclosure headers): contributes button semantics and nothing else. Ordinary actions
// belong to Button / IconButton / TextButton, which own metrics and presentation.
export function Pressable({ children, ...props }: PressableProps) {
  return <ButtonPrimitive {...props}>{children}</ButtonPrimitive>;
}
