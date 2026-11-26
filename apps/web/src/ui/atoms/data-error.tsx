import { iconGlyph, type IconName } from "@/ui/icons/index";
import { type CSSProperties, type RefObject } from "react";
import { useT } from "@/lib/i18n/index";
import { isUnsupportedMethod } from "@/lib/rpcErrors";
import { handoffFocus } from "@flame/ui";
import { Button } from "@flame/ui/button";
import { EmptyState, type EmptyStateAlign, type EmptyStateSize } from "@flame/ui/empty-state";
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
  retryLabel?: string;
  title?: string;
  sub?: string;
  size?: EmptyStateSize;
  align?: EmptyStateAlign;
  style?: CSSProperties;
}

export function DataError({
  failure,
  unsupported,
  focusTarget,
  onRetry,
  retryLabel,
  title,
  sub,
  size,
  align,
  style,
}: Props) {
  const t = useT();
  if (failure != null && isUnsupportedMethod(failure)) {
    return (
      <EmptyState
        icon={iconGlyph(unsupported?.icon ?? "alert")}
        tone="warning"
        title={unsupported?.title ?? t("runtime.unsupported.title")}
        sub={unsupported?.sub ?? t("runtime.unsupported.sub")}
        size={unsupported?.size}
        align={align}
        style={style}
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
      align={align}
      style={style}
      action={
        onRetry && (
          <Button
            variant="outline"
            size={size === "compact" ? "xs" : "sm"}
            onClick={(event) => {
              const retiring = event.currentTarget;
              handoffFocus(
                retiring,
                focusTarget?.current ?? retiring.parentElement?.closest<HTMLElement>("[tabindex]"),
              );
              onRetry();
            }}
          >
            {retryLabel ?? t("common.retry")}
          </Button>
        )
      }
    />
  );
}
