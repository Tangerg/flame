import type { ComponentPropsWithoutRef, ReactNode, Ref } from "react";
import { cn } from "@/lib/classNames";

export function AgentComposerSurface({
  className,
  children,
  ...props
}: ComponentPropsWithoutRef<"div">) {
  return (
    <div
      {...props}
      className={cn(
        "agent-composer-glass overflow-hidden rounded-composer",
        "transition-[box-shadow] duration-[var(--dur-med)] ease-out",
        className,
      )}
    >
      {children}
    </div>
  );
}

/** The chip row under the input. Owns the density padding and the control-size overrides the
 *  chips read, so a chip stays the composer's size wherever it is contributed from. `labelled`
 *  is the fitted state; its `data-measuring` companion is toggled on the ref for the length of
 *  the measuring reflow, which is why that one is not a prop. */
export function AgentComposerFooter({
  labelled,
  ref,
  children,
}: {
  labelled: boolean;
  ref?: Ref<HTMLDivElement>;
  children: ReactNode;
}) {
  return (
    <div
      ref={ref}
      data-slot="composer-footer"
      data-labelled={labelled ? "" : undefined}
      className="agent-composer-footer flex flex-nowrap items-center gap-1.5 pr-[var(--density-composer-footer-end)] pb-[var(--density-composer-footer)] pl-[var(--density-composer-footer)]"
    >
      {children}
    </div>
  );
}
