// UI-surface selectors — layout slots, Work Index items, workspace views and
// settings panes.

import { useMemo } from "react";
import type {
  LayoutSlotSpec,
  SettingsPaneSpec,
  WorkIndexItemSpec,
  WorkspaceViewSpec,
} from "../types";
import { LAYOUT_SLOT, SETTINGS_PANE, WORK_INDEX_ITEM, WORKSPACE_VIEW } from "../kernelPoints";
import { useContributions } from "../kernel";
import { createPointSubIndex, useExtensionPoint } from "./extensions";

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

export function useWorkspaceViews(): WorkspaceViewSpec[] {
  return useExtensionPoint(WORKSPACE_VIEW);
}

export function useWorkIndexItems(): WorkIndexItemSpec[] {
  return useExtensionPoint(WORK_INDEX_ITEM);
}

export function useSettingsPanes(): SettingsPaneSpec[] {
  return useExtensionPoint(SETTINGS_PANE);
}
