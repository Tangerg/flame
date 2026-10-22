import { iconGlyph } from "@/ui/icons/index";
import { useT } from "@/lib/i18n/index";
import { Button } from "@flame/ui/button";
import { EmptyState, type EmptyStateSize } from "@flame/ui/empty-state";
interface Props {
  onRetry?: () => void;
  title?: string;
  sub?: string;
  size?: EmptyStateSize;
}

export function DataError({ onRetry, title, sub, size }: Props) {
  const t = useT();
  return (
    <EmptyState
      icon={iconGlyph("alert")}
      tone="negative"
      title={title ?? t("dataView.error.title")}
      sub={sub ?? t("dataView.error.sub")}
      size={size}
      action={
        onRetry && (
          <Button variant="outline" size={size === "compact" ? "xs" : "sm"} onClick={onRetry}>
            {t("common.retry")}
          </Button>
        )
      }
    />
  );
}
