import { DOCK_MIN_WIDTH_PX, dockRatioFromWidth, maxDockWidth } from "@/lib/shellGeometry";
import { useT } from "@/lib/i18n";
import { AgentDockResizer, DOCK_MEASURE_PROPERTY } from "@/ui/agent";
import { useDockWidth } from "@/plugins/builtin/workspace/public/sidebarDrawer";

const dockFloor = () => DOCK_MIN_WIDTH_PX;

export function DockResizer({ width }: { width: number }) {
  const t = useT();
  const { setWidth: setRatio } = useDockWidth();

  return (
    <AgentDockResizer
      aria-label={t("dock.action.resize")}
      value={width}
      container={(rail) => rail.parentElement}
      property={DOCK_MEASURE_PROPERTY}
      read={() => width}
      minWidth={dockFloor}
      maxWidth={maxDockWidth}
      onCommit={(width, rowWidth) => setRatio(dockRatioFromWidth(width, rowWidth))}
      resizingAttribute="data-resizing"
    />
  );
}
