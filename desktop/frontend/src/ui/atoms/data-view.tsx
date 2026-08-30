import type { ReactNode } from "react";
import type { IconName } from "@/ui/icons";
import { useT } from "@/lib/i18n";
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
  /** Without this a rejected query falls through to the empty branch and masks a hard
   *  error as "nothing here yet". */
  isError?: boolean;
  skeletonCount?: number;
  skeletonVariant?: SkeletonListVariant;
  loadingLabel?: string;
  empty?: EmptyConfig;
  error?: EmptyConfig;
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
  error,
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
  if (isError) {
    return (
      <EmptyState
        icon="alert"
        title={t("dataView.error.title")}
        sub={t("dataView.error.sub")}
        {...error}
      />
    );
  }
  if (!items || items.length === 0) {
    return empty ? <EmptyState {...empty} /> : null;
  }
  return <>{children(items)}</>;
}
