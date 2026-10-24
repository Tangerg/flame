import { iconGlyph } from "@/ui/icons/index";
import { type ReactNode } from "react";
import { type IconName } from "@/ui/icons/index";
import { useT } from "@/lib/i18n/index";
import { EmptyState, type EmptyStateSize } from "@flame/ui/empty-state";
import { DataError, type UnsupportedConfig } from "@/ui/atoms/data-error";
import { SkeletonList, type SkeletonListVariant } from "@/ui/atoms/skeleton";
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
  unsupported?: UnsupportedConfig;
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
  if (failure != null) {
    return <DataError failure={failure} unsupported={unsupported} onRetry={onRetry} {...error} />;
  }
  if (!items || items.length === 0) {
    return empty ? <EmptyState {...empty} icon={iconGlyph(empty.icon)} /> : null;
  }
  return <>{children(items)}</>;
}
