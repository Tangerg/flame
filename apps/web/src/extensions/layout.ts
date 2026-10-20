import { type ComponentType } from "react";
import { defineExtensionPoint } from "@/extensions/contracts";
import { useMemo } from "react";
import { useContributions } from "@/extensions/kernel";
import { createPointSubIndex } from "@/extensions/selectors/extensions";
export interface LayoutSlotSpec {
  id: string;
  order?: number;
  className?: string;
  component: ComponentType;
}

export const LAYOUT_SLOT = defineExtensionPoint<{ slot: string; spec: LayoutSlotSpec }>({
  id: "flame.layoutSlot",
  keying: "multi",
});

const layoutBySlot = createPointSubIndex((item: { slot: string; spec: LayoutSlotSpec }) => ({
  key: item.slot,
  value: item.spec,
}));

export function useLayoutSlot(slot: string): LayoutSlotSpec[] {
  const entries = useContributions(LAYOUT_SLOT);
  return useMemo(
    () =>
      [...(layoutBySlot(entries).get(slot) ?? [])].sort(
        (a, b) => (a.order ?? 100) - (b.order ?? 100),
      ),
    [entries, slot],
  );
}
