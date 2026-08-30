import { describe, expect, it } from "vitest";
import {
  canPresentDock,
  clampDockWidth,
  clampSidebarWidth,
  defaultDockWidth,
  DOCK_MIN_WIDTH_PX,
  DOCK_SAFE_AREA_PX,
  dockRatioFromWidth,
  dockWidthFromRatio,
  maxDockWidth,
  maxSidebarWidth,
  minDockWidth,
  SIDEBAR_DEFAULT_WIDTH_PX,
  shouldReclaimDrawer,
  SIDEBAR_MIN_WIDTH_PX,
} from "./shellGeometry";

describe("sidebar geometry", () => {
  it("matches the Codex desktop default and bounded resize range", () => {
    expect(SIDEBAR_DEFAULT_WIDTH_PX).toBe(275);
    expect(maxSidebarWidth(1440)).toBe(520);
    expect(clampSidebarWidth(900, 1440)).toBe(520);
    expect(clampSidebarWidth(320, 1440)).toBe(320);
  });

  it("keeps both the drawer and reading plane operable in a narrow window", () => {
    expect(maxSidebarWidth(720)).toBe(480);
    expect(clampSidebarWidth(900, 720)).toBe(480);
    expect(clampSidebarWidth(100, 720)).toBe(SIDEBAR_MIN_WIDTH_PX);
  });
});

describe("dock geometry", () => {
  it("reserves the conversation's safe area before the flank may claim anything", () => {
    expect(maxDockWidth(1120)).toBe(1120 - DOCK_SAFE_AREA_PX);
    expect(clampDockWidth(2000, 1120)).toBe(768);
    expect(clampDockWidth(420, 1120)).toBe(420);
    expect(clampDockWidth(100, 1120)).toBe(DOCK_MIN_WIDTH_PX);
  });

  it("collapses the range onto the floor rather than inverting it", () => {
    expect(maxDockWidth(400)).toBe(DOCK_MIN_WIDTH_PX);
    expect(minDockWidth(400)).toBe(DOCK_MIN_WIDTH_PX);
    expect(clampDockWidth(1000, 400)).toBe(DOCK_MIN_WIDTH_PX);
  });

  it("folds the dock when the row cannot hold the floor beside the safe area", () => {
    expect(canPresentDock(DOCK_MIN_WIDTH_PX + DOCK_SAFE_AREA_PX - 1)).toBe(false);
    expect(canPresentDock(DOCK_MIN_WIDTH_PX + DOCK_SAFE_AREA_PX)).toBe(true);
  });

  it("round-trips a position in the range through a ratio", () => {
    const rowWidth = 1440;
    for (const width of [320, 500, 768, 1088]) {
      expect(dockWidthFromRatio(dockRatioFromWidth(width, rowWidth), rowWidth)).toBe(width);
    }
    expect(dockWidthFromRatio(0, rowWidth)).toBe(minDockWidth(rowWidth));
    expect(dockWidthFromRatio(1, rowWidth)).toBe(maxDockWidth(rowWidth));
  });

  it("keeps a stored ratio meaningful when the row changes size", () => {
    const ratio = dockRatioFromWidth(768, 1440);
    expect(dockWidthFromRatio(ratio, 1440)).toBe(768);
    expect(dockWidthFromRatio(ratio, 1920)).toBeGreaterThan(768);
    expect(dockWidthFromRatio(ratio, 1100)).toBeLessThan(768);
  });

  it("answers a safe ratio for a degenerate range or a broken stored value", () => {
    expect(dockRatioFromWidth(500, 400)).toBe(1);
    expect(dockWidthFromRatio(Number.NaN, 1440)).toBe(maxDockWidth(1440));
  });

  it("opens at the widest measure every claim allows", () => {
    expect(defaultDockWidth(1440, 900)).toBe(940);
    expect(defaultDockWidth(1440, 500)).toBe(800);
    expect(defaultDockWidth(800, 900)).toBe(448);
    expect(defaultDockWidth(500, 900)).toBe(DOCK_MIN_WIDTH_PX);
  });
});

describe("what a narrow window reclaims", () => {
  it("takes the drawer's measure back before the flank has to fold", () => {
    expect(shouldReclaimDrawer(1120, 275)).toBe(false);
    expect(shouldReclaimDrawer(1120, 520)).toBe(true);
    expect(shouldReclaimDrawer(912, 240)).toBe(false);
    expect(shouldReclaimDrawer(900, 240)).toBe(true);
  });

  it("measures against the drawer the window would actually draw", () => {
    expect(shouldReclaimDrawer(1120, 9000)).toBe(shouldReclaimDrawer(1120, 520));
  });
});
