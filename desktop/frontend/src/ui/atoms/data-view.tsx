import type { ReactNode } from "react";
import type { IconName } from "@/ui/icons";
import { useT } from "@/lib/i18n";
import { Button } from "./button";
import { EmptyState } from "./empty-state";
import { SkeletonList, type SkeletonListVariant } from "./skeleton";

interface EmptyConfig {
  icon?: IconName;
  title: string;
  sub?: string;
  size?: "compact" | "comfortable";
}

interface Props<T> {
  items: T[] | undefined;
  isLoading: boolean;
  isError?: boolean;
  skeletonCount?: number;
  skeletonVariant?: SkeletonListVariant;
  loadingLabel?: string;
  empty?: EmptyConfig;
  /** The Runtime does not implement this call. It reads as a limit rather than a failure and
   *  offers no retry, because there is nothing to try again. */
  unsupported?: EmptyConfig;
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
  isError,
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
  if (unsupported) return <EmptyState {...unsupported} />;
  if (isError) {
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
