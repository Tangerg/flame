import {
  clampDockWidth,
  dockRatioFromWidth,
  dockWidthFromRatio,
  maxDockWidth,
  minDockWidth,
} from "@/lib/shellGeometry";
import { useT } from "@/lib/i18n";
import { ResizeHandle } from "@/ui";
import { useDockWidth } from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { DOCK_RATIO_PROPERTY } from "./dockWidth";

export function DockResizer() {
  const t = useT();
  const { width: ratio, setWidth: setRatio } = useDockWidth();

  return (
    <ResizeHandle
      aria-label={t("dock.action.resize")}
      className="agent-pane-resizer"
      edge="start"
      value={ratio ?? 1}
      container={(rail) => rail.parentElement}
      property={DOCK_RATIO_PROPERTY}
      read={readDockWidth}
      minWidth={minDockWidth}
      maxWidth={maxDockWidth}
      formatProperty={(width, rowWidth) => String(dockRatioFromWidth(width, rowWidth))}
      onCommit={(width, rowWidth) => setRatio(dockRatioFromWidth(width, rowWidth))}
    />
  );
}

function readDockWidth(row: HTMLElement): number {
  const dock = row.querySelector<HTMLElement>(".agent-context-dock");
  const renderedWidth = dock?.getBoundingClientRect().width ?? 0;
  if (renderedWidth > 0) return clampDockWidth(renderedWidth, row.clientWidth);
  const storedRatio = Number.parseFloat(
    getComputedStyle(row).getPropertyValue(DOCK_RATIO_PROPERTY),
  );
  return dockWidthFromRatio(storedRatio, row.clientWidth);
}
