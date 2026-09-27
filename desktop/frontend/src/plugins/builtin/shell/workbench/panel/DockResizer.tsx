import {
  clampDockWidth,
  DOCK_MIN_WIDTH_PX,
  dockRatioFromWidth,
  defaultDockWidth,
  maxDockWidth,
} from "@/lib/shellGeometry";
import { useT } from "@/lib/i18n";
import { AgentDockResizer, agentDockElement } from "@/ui/agent";
import { useDockWidth } from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { DOCK_MEASURE_PROPERTY, dockWidthMeasure } from "./dockWidth";

const dockFloor = () => DOCK_MIN_WIDTH_PX;

export function DockResizer() {
  const t = useT();
  const { width: ratio, setWidth: setRatio } = useDockWidth();

  return (
    <AgentDockResizer
      aria-label={t("dock.action.resize")}
      value={ratio ?? 1}
      container={(rail) => rail.parentElement}
      property={DOCK_MEASURE_PROPERTY}
      read={readDockWidth}
      minWidth={dockFloor}
      maxWidth={maxDockWidth}
      formatProperty={(width, rowWidth) => dockWidthMeasure(dockRatioFromWidth(width, rowWidth))}
      onCommit={(width, rowWidth) => setRatio(dockRatioFromWidth(width, rowWidth))}
    />
  );
}

function readDockWidth(row: HTMLElement): number {
  const dock = agentDockElement(row);
  const renderedWidth = dock?.getBoundingClientRect().width ?? 0;
  if (renderedWidth > 0) return clampDockWidth(renderedWidth, row.clientWidth);
  return defaultDockWidth(row.clientWidth);
}
