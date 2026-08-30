import { cn } from "@/lib/classNames";
import { IconButton, type IconName } from "@/ui";

// `title` is optional for the one action that opens a menu: that trigger must BE the
// element the menu anchors to, so it composes this as its rendered element and puts the
// tooltip outside.
interface MessageActionButtonProps {
  icon: IconName;
  role: string;
  title?: string;
  onClick?: () => void;
  className?: string;
  "aria-label"?: string;
  "aria-pressed"?: boolean;
}

export function MessageActionButton({ role, className, ...props }: MessageActionButtonProps) {
  return (
    <IconButton
      {...props}
      iconSize="sm"
      // `sm`, not `xs`: xs is 24px nominal and lands at 22 once a view scales its density,
      // and four buttons butted together have no spacing exemption — not worth trading the
      // WCAG target-size floor for four pixels.
      size="sm"
      quiet
      className={cn(role === "user" ? "rounded-full" : "rounded-md", className)}
    />
  );
}
