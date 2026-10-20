import { type ReactNode } from "react";
import { ButtonPrimitive, type ButtonPrimitiveProps } from "@flame/ui/primitives";
export type PressableProps = Omit<ButtonPrimitiveProps, "children"> & {
  children?: ReactNode;
};

export function Pressable({ children, ...props }: PressableProps) {
  return <ButtonPrimitive {...props}>{children}</ButtonPrimitive>;
}
