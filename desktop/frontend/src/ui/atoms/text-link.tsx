import type { ReactNode } from "react";
import { AnchorPrimitive, type AnchorPrimitiveProps } from "@/ui/primitives";

export type TextLinkProps = Omit<AnchorPrimitiveProps, "children" | "rel" | "target"> & {
  children: ReactNode;
};

export function TextLink({ children, href, ...props }: TextLinkProps) {
  const external = href && !href.startsWith("#");
  return (
    <AnchorPrimitive
      {...props}
      href={href || undefined}
      target={external ? "_blank" : undefined}
      rel={external ? "noopener noreferrer" : undefined}
    >
      {children}
    </AnchorPrimitive>
  );
}
