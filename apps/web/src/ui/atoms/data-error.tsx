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
          <Button
            variant="outline"
            size={size === "compact" ? "xs" : "sm"}
            onClick={(event) => {
              // Retrying replaces this error, so hand owned focus to its surviving host first.
              const retiring = event.currentTarget;
              if (retiring === retiring.ownerDocument.activeElement) {
                retiring.parentElement
                  ?.closest<HTMLElement>("[tabindex]")
                  ?.focus({ preventScroll: true });
              }
              onRetry();
            }}
          >
            {t("common.retry")}
          </Button>
        )
      }
    />
  );
}
