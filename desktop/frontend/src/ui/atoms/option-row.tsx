import { cva, type VariantProps } from "class-variance-authority";
import { cn } from "@/lib/classNames";
import { Pressable, type PressableProps } from "./pressable";

// Two mechanisms answer "which row is the keyboard on" — Base UI's `data-highlighted` and
// a hand-driven listbox's `selected`. Both wash the row here so the choice of behaviour
// library cannot change how selection looks.
export const floatingRowStyles = cva(
  [
    "w-full items-center gap-2 rounded-[var(--shape-sm)] border-0 bg-transparent px-2 text-left",
    "text-ui-md text-fg outline-none transition-colors",
    // Hover first so a selected row that is also hovered stays selected.
    "hover:bg-hover",
    "aria-selected:bg-selected data-[highlighted]:bg-hover",
  ].join(" "),
  {
    variants: {
      // `grid` where rows align columns with each other and the consumer names the
      // template; `flex` where trailing pieces are optional and no template can describe
      // a cell that may not be there.
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
    /**
     * Passing this also makes the row an `option`, since `role="option"` requires
     * `aria-selected`. Leave UNSET where a behaviour library owns selection — Base UI
     * sets its own attributes and `undefined` here must not overwrite them.
     */
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
