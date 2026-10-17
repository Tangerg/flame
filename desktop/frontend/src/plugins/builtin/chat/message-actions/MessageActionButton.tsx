import { iconGlyph, type IconName } from "@/ui/icons";
import { IconButton, type ButtonTone } from "@flame/ui";

interface MessageActionButtonProps {
  icon: IconName;
  role: string;
  title?: string;
  onClick?: () => void;
  tone?: ButtonTone;
  className?: string;
  "aria-label": string;
  "aria-pressed"?: boolean;
}

export function MessageActionButton({ role, className, icon, ...props }: MessageActionButtonProps) {
  const isUser = role === "user";
  return (
    <IconButton
      {...props}
      icon={iconGlyph(icon)}
      size="sm"
      quiet
      round={isUser}
      className={className}
    />
  );
}
