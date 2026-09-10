import type { IconName } from "@/ui";
import { Button, Icon } from "@/ui";

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
  onClick: () => void;
  primary?: boolean;
  tone?: "negative" | "warning";
  disabled?: boolean;
  pending?: boolean;
}) {
  return (
    <Button
      size="xs"
      variant={primary ? "tonal" : "soft"}
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
