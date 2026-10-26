import { type ReactNode, type ComponentPropsWithRef } from "react";
export type TextLinkProps = Omit<ComponentPropsWithRef<"a">, "children" | "rel" | "target"> & {
  children: ReactNode;
};

export function TextLink({ children, href, ...props }: TextLinkProps) {
  const external = href && !href.startsWith("#");
  return (
    <a
      {...props}
      href={href || undefined}
      target={external ? "_blank" : undefined}
      rel={external ? "noopener noreferrer" : undefined}
    >
      {children}
    </a>
  );
}
