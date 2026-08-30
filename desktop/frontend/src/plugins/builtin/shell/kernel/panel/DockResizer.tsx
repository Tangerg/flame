import { clampDockWidth, DOCK_MIN_WIDTH_PX, maxDockWidth } from "@/lib/shellGeometry";
import { useT } from "@/lib/i18n";
import { ResizeHandle } from "@/ui";
import { useDockWidth } from "@/plugins/builtin/workspace/public/sidebarDrawer";
import { DOCK_WIDTH_PROPERTY } from "./dockWidth";

export function DockResizer() {
  const t = useT();
  const { width, setWidth } = useDockWidth();

  return (
    <ResizeHandle
      aria-label={t("dock.action.resize")}
      className="agent-pane-resizer"
      edge="start"
      value={width}
      container={(rail) => rail.parentElement}
      property={DOCK_WIDTH_PROPERTY}
      read={readDockWidth}
      minWidth={DOCK_MIN_WIDTH_PX}
      maxWidth={maxDockWidth}
      onCommit={setWidth}
    />
  );
}

function readDockWidth(row: HTMLElement): number {
  const dock = row.querySelector<HTMLElement>(".agent-context-dock");
  const renderedWidth = dock?.getBoundingClientRect().width ?? 0;
  if (renderedWidth > 0) return clampDockWidth(renderedWidth, row.clientWidth);
  const propertyWidth = Number.parseFloat(
    getComputedStyle(row).getPropertyValue(DOCK_WIDTH_PROPERTY),
  );
  return clampDockWidth(Number.isFinite(propertyWidth) ? propertyWidth : 0, row.clientWidth);
}
