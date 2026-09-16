import type { ReactNode } from "react";
import type { IconName } from "@/ui/icons";
import { useT } from "@/lib/i18n";
import { isUnsupportedMethod } from "@/lib/rpcErrors";
import { Button } from "./button";
import { EmptyState, type EmptyStateSize } from "./empty-state";
import { SkeletonList, type SkeletonListVariant } from "./skeleton";

interface EmptyConfig {
  icon?: IconName;
  title: string;
  sub?: string;
  size?: EmptyStateSize;
}

interface Props<T> {
  items: T[] | undefined;
  isLoading: boolean;
  /**
   * What went wrong, not merely that something did.
   *
   * The view cannot tell a Runtime that FAILED from one that never implemented the call unless
   * it is handed the failure, and the two states are not the same offer: three call sites
   * classified it themselves and seventeen did not, so seventeen answered a method the Runtime
   * does not have with a Retry that could never succeed.
   */
  failure?: unknown;
  skeletonCount?: number;
  skeletonVariant?: SkeletonListVariant;
  loadingLabel?: string;
  empty?: EmptyConfig;
  /** Wording and glyph for a call the Runtime does not implement, when this view can say
   *  something more exact than the standing line. The STATE is recognised from `failure`; this
   *  only dresses it. */
  unsupported?: Partial<EmptyConfig>;
  /** The glyph is this owner's: an error that draws itself with the view's own icon is the
   *  same picture as that view's empty result, and the two states then differ only in wording. */
  error?: Omit<EmptyConfig, "icon">;
  /** Queries default to one retry and never refetch on focus, so without this an error state
   *  is terminal — the only way back is to unmount the view and return to it. */
  onRetry?: () => void;
  children: (items: T[]) => ReactNode;
}

export function DataView<T>({
  items,
  isLoading,
  failure,
  skeletonCount = 4,
  skeletonVariant = "stacked",
  loadingLabel,
  empty,
  unsupported,
  error,
  onRetry,
  children,
}: Props<T>) {
  const t = useT();
  if (isLoading) {
    return (
      <SkeletonList
        count={skeletonCount}
        variant={skeletonVariant}
        label={loadingLabel ?? t("common.loading")}
      />
    );
  }
  if (failure != null && isUnsupportedMethod(failure)) {
    // No retry: a call the Runtime does not have is a limit, and there is nothing to try again.
    return (
      <EmptyState
        icon={unsupported?.icon ?? "alert"}
        title={unsupported?.title ?? t("runtime.unsupported.title")}
        sub={unsupported?.sub ?? t("runtime.unsupported.sub")}
        size={unsupported?.size}
      />
    );
  }
  if (failure != null) {
    return (
      <EmptyState
        icon="alert"
        title={t("dataView.error.title")}
        sub={t("dataView.error.sub")}
        {...error}
        action={
          onRetry && (
            <Button
              variant="outline"
              size={error?.size === "compact" ? "xs" : "sm"}
              onClick={onRetry}
            >
              {t("common.retry")}
            </Button>
          )
        }
      />
    );
  }
  if (!items || items.length === 0) {
    return empty ? <EmptyState {...empty} /> : null;
  }
  return <>{children(items)}</>;
}
