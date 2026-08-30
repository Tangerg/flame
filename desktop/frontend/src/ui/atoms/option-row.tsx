import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/classNames";
import { Pressable, type PressableProps } from "./pressable";

export const floatingRowStyles = cva(
  [
    "w-full items-center gap-2 rounded-[var(--shape-sm)] border-0 bg-transparent px-2 text-left",
    "text-ui-md text-fg outline-none transition-colors",
    "hover:bg-hover",
    "aria-selected:bg-selected data-[highlighted]:bg-hover",
  ].join(" "),
  {
    variants: {
      layout: { grid: "grid", flex: "flex" },
      size: {
        sm: "min-h-[var(--menu-row-height)] py-px",
        md: "h-8",
        lg: "min-h-9 py-1.5",
      },
    },
    defaultVariants: { layout: "grid", size: "md" },
  },
);

export type OptionRowProps = Omit<PressableProps, "aria-selected"> &
  VariantProps<typeof floatingRowStyles> & {
    selected?: boolean;
  };

export function OptionRow({ layout, size, selected, className, ...props }: OptionRowProps) {
  return (
    <Pressable
      {...props}
      type={props.type ?? "button"}
      {...(selected === undefined
        ? {}
        : { role: props.role ?? "option", "aria-selected": selected })}
      className={cn(floatingRowStyles({ layout, size }), className)}
    />
  );
}
