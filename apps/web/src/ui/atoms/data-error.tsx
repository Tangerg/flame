import { iconGlyph, type IconName } from "@/ui/icons/index";
import { type RefObject } from "react";
import { useT } from "@/lib/i18n/index";
import { isUnsupportedMethod } from "@/lib/rpcErrors";
import { Button } from "@flame/ui/button";
import { EmptyState, type EmptyStateSize } from "@flame/ui/empty-state";
export interface UnsupportedConfig {
  icon?: IconName;
  title?: string;
  sub?: string;
  size?: EmptyStateSize;
}

interface Props {
  failure?: unknown;
  unsupported?: UnsupportedConfig;
  focusTarget?: RefObject<HTMLElement | null>;
  onRetry?: () => void;
  title?: string;
  sub?: string;
  size?: EmptyStateSize;
}

export function DataError({ failure, unsupported, focusTarget, onRetry, title, sub, size }: Props) {
  const t = useT();
  if (failure != null && isUnsupportedMethod(failure)) {
    return (
      <EmptyState
        icon={iconGlyph(unsupported?.icon ?? "alert")}
        tone="warning"
        title={unsupported?.title ?? t("runtime.unsupported.title")}
        sub={unsupported?.sub ?? t("runtime.unsupported.sub")}
        size={unsupported?.size}
      />
    );
  }
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
                const host =
                  focusTarget?.current ??
                  retiring.parentElement?.closest<HTMLElement>("[tabindex]");
                host?.focus({ preventScroll: true });
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
