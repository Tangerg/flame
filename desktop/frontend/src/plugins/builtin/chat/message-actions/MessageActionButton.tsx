import { IconButton, type IconName, type ButtonTone } from "@/ui";

interface MessageActionButtonProps {
  icon: IconName;
  role: string;
  title?: string;
  onClick?: () => void;
  tone?: ButtonTone;
  className?: string;
  "aria-label"?: string;
  "aria-pressed"?: boolean;
}

export function MessageActionButton({ role, className, ...props }: MessageActionButtonProps) {
  const isUser = role === "user";
  return <IconButton {...props} size="sm" quiet round={isUser} className={className} />;
}
