import { describe, expect, it } from "vitest";
import { notificationInsets } from "@/features/shell/workbench/panel/notificationSpace";

describe("notification reading space", () => {
  it("keeps bottom-anchored composing controls outside the reading band", () => {
    expect(notificationInsets({ top: 46, bottom: 525 }, { top: 373, bottom: 509 }, 525)).toEqual({
      top: 46,
      bottom: 152,
    });
  });

  it("uses the larger band below a centered composer", () => {
    expect(notificationInsets({ top: 46, bottom: 600 }, { top: 180, bottom: 300 }, 600)).toEqual({
      top: 300,
      bottom: 0,
    });
  });

  it("keeps equal bands above the composer", () => {
    expect(notificationInsets({ top: 100, bottom: 500 }, { top: 250, bottom: 350 }, 600)).toEqual({
      top: 100,
      bottom: 350,
    });
  });

  it("never claims space outside the viewport or beyond a fully occupied layout", () => {
    expect(notificationInsets({ top: -20, bottom: 700 }, { top: -40, bottom: 900 }, 600)).toEqual({
      top: 0,
      bottom: 600,
    });
    expect(notificationInsets({ top: -20, bottom: 700 }, { top: 400, bottom: 900 }, 600)).toEqual({
      top: 0,
      bottom: 200,
    });
  });

  it("keeps the layout band when an empty-pane composer scrolls past either edge", () => {
    expect(notificationInsets({ top: 100, bottom: 400 }, { top: 450, bottom: 500 }, 500)).toEqual({
      top: 100,
      bottom: 100,
    });
    expect(notificationInsets({ top: 100, bottom: 400 }, { top: 0, bottom: 50 }, 500)).toEqual({
      top: 100,
      bottom: 100,
    });
  });
});
