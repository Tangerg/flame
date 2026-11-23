import { type IconName } from "@/ui/index";
import { Icon } from "@/ui/index";
import { Button } from "@flame/ui";
import { type MouseEventHandler } from "react";
export function BannerAction({
  icon,
  label,
  onClick,
  primary,
  tone = "negative",
  disabled,
  pending,
}: {
  icon?: IconName;
  label: string;
  onClick: MouseEventHandler<HTMLButtonElement>;
  primary?: boolean;
  tone?: "negative" | "warning";
  disabled?: boolean;
  pending?: boolean;
}) {
  // One filled recovery action; the alternatives stay quiet so the banner does not read as tags.
  return (
    <Button
      size="xs"
      variant={primary ? "tonal" : "ghost"}
      tone={primary ? tone : undefined}
      onClick={onClick}
      disabled={disabled}
      pending={pending}
    >
      {icon && <Icon name={icon} size="xs" />}
      <span>{label}</span>
    </Button>
  );
}
