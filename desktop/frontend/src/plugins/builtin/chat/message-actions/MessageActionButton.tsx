import { cn } from "@/lib/classNames";
import { IconButton, type IconName } from "@/ui";

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
      size="sm"
      quiet
      round={role === "user"}
      className={cn(role !== "user" && "rounded-md", className)}
    />
  );
}
