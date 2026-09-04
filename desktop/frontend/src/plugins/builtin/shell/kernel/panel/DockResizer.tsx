import {
  clampDockWidth,
  DOCK_MIN_WIDTH_PX,
  dockRatioFromWidth,
  dockWidthFromRatio,
  maxDockWidth,
} from "@/lib/shellGeometry";
import { useT } from "@/lib/i18n";
import { AgentDockResizer, agentDockElement } from "@/ui/agent";
import { useDockWidth } from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { DOCK_RATIO_PROPERTY } from "./dockWidth";

// The floor does not vary with the row: `maxDockWidth` already refuses to fall below it.
const dockFloor = () => DOCK_MIN_WIDTH_PX;

export function DockResizer() {
  const t = useT();
  const { width: ratio, setWidth: setRatio } = useDockWidth();

  return (
    <AgentDockResizer
      aria-label={t("dock.action.resize")}
      value={ratio ?? 1}
      container={(rail) => rail.parentElement}
      property={DOCK_RATIO_PROPERTY}
      read={readDockWidth}
      minWidth={dockFloor}
      maxWidth={maxDockWidth}
      formatProperty={(width, rowWidth) => String(dockRatioFromWidth(width, rowWidth))}
      onCommit={(width, rowWidth) => setRatio(dockRatioFromWidth(width, rowWidth))}
    />
  );
}

function readDockWidth(row: HTMLElement): number {
  const dock = agentDockElement(row);
  const renderedWidth = dock?.getBoundingClientRect().width ?? 0;
  if (renderedWidth > 0) return clampDockWidth(renderedWidth, row.clientWidth);
  const storedRatio = Number.parseFloat(
    getComputedStyle(row).getPropertyValue(DOCK_RATIO_PROPERTY),
  );
  return dockWidthFromRatio(storedRatio, row.clientWidth);
}
