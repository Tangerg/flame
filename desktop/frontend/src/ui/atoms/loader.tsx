import { useT } from "@/lib/i18n";
import { cn } from "@/lib/classNames";

type LoaderSize = "sm" | "md" | "lg";

export interface LoaderProps {
  size?: LoaderSize;
  text?: string;
  className?: string;
}

const TEXT: Record<LoaderSize, string> = {
  sm: "text-ui-xs",
  md: "text-ui-sm",
  lg: "text-ui-md",
};

/** The label a reader cannot see is the one a screen reader needs: the shimmer is decoration,
 *  and `role="status"` on `<output>` is what actually announces that something is running. */
function Announcement() {
  const t = useT();
  return <output className="sr-only">{t("common.loading")}</output>;
}

/**
 * Waiting, said one way. This carried seven variants — dots, typing, pulse-dot, wave, bars,
 * terminal, shimmer — and both call sites in the tree asked for the shimmer, so six of them
 * and five sets of keyframes animated nothing. No guard could see it: a `variant` the product
 * never passes is still reachable through the union, so `knip` reads the component as used and
 * `check-dead-utilities` reads every class as emitted.
 *
 * A variant this needs again is one rung to add back, which is cheaper than six kept warm.
 */
export function Loader({ size = "md", text = "Thinking", className }: LoaderProps) {
  return (
    <span
      className={cn(
        "inline-block bg-clip-text font-medium text-transparent animate-shimmer motion-reduce:animate-none",
        "bg-[linear-gradient(90deg,var(--color-text-muted)_35%,var(--color-text)_50%,var(--color-text-muted)_65%)]",
        "bg-[length:200%_100%]",
        TEXT[size],
        className,
      )}
    >
      {text}
      <Announcement />
    </span>
  );
}
