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
  failure?: unknown;
  skeletonCount?: number;
  skeletonVariant?: SkeletonListVariant;
  loadingLabel?: string;
  empty?: EmptyConfig;
  unsupported?: Partial<EmptyConfig>;
  error?: Omit<EmptyConfig, "icon">;
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
