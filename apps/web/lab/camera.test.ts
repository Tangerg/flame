import { afterEach, describe, expect, it, vi } from "vitest";
import { Camera, CAMERA_LIMITS, readCamera, saveCamera } from "./camera";

afterEach(() => vi.restoreAllMocks());

const pairedBoards = { width: 2592, height: 936 };
const narrowCanvas = { width: 320, height: 240 };

describe("component canvas camera", () => {
  it("pans without changing the previous camera or scale", () => {
    const camera = Camera.at(20, -40, 0.8);
    const next = camera.pan({ x: 30, y: -15 });
    expect(next).toMatchObject({ x: 50, y: -55, scale: 0.8 });
    expect(camera).toMatchObject({ x: 20, y: -40, scale: 0.8 });
  });

  it.each([0.1, 0.5, 1.2, 10])(
    "keeps the world point under the cursor when zooming by %s",
    (factor) => {
      const camera = Camera.at(-120, 45, 0.75);
      const anchor = { x: 310, y: 180 };
      const world = {
        x: (anchor.x - camera.x) / camera.scale,
        y: (anchor.y - camera.y) / camera.scale,
      };
      const next = camera.zoom(factor, anchor);
      expect((anchor.x - next.x) / next.scale).toBeCloseTo(world.x);
      expect((anchor.y - next.y) / next.scale).toBeCloseTo(world.y);
      expect(next.scale).toBeGreaterThanOrEqual(CAMERA_LIMITS.minimumScale);
      expect(next.scale).toBeLessThanOrEqual(CAMERA_LIMITS.maximumScale);
    },
  );

  it("bounds offsets and scale at construction and through gestures", () => {
    const maximum = CAMERA_LIMITS.maximumOffset;
    const bounded = Camera.at(maximum * 2, -maximum * 2, CAMERA_LIMITS.maximumScale * 2);
    expect(bounded).toMatchObject({ x: maximum, y: -maximum, scale: CAMERA_LIMITS.maximumScale });
    expect(bounded.pan({ x: maximum, y: -maximum })).toMatchObject({ x: maximum, y: -maximum });
    expect(Camera.at(0, 0, -1).scale).toBe(CAMERA_LIMITS.minimumScale);
  });

  it.each([NaN, Infinity, -Infinity])(
    "rejects nonfinite camera or gesture coordinates: %s",
    (value) => {
      expect(() => Camera.at(value, 0)).toThrow(/finite/);
      expect(() => Camera.at(0, value)).toThrow(/finite/);
      expect(() => Camera.at(0, 0, value)).toThrow(/finite/);
      expect(() => Camera.at().pan({ x: value, y: 0 })).toThrow(/finite/);
      expect(() => Camera.at().zoom(1.2, { x: 0, y: value })).toThrow(/finite/);
    },
  );

  it.each([0, -1, NaN, Infinity])("rejects an invalid zoom factor: %s", (factor) => {
    expect(() => Camera.at().zoom(factor, { x: 0, y: 0 })).toThrow(/zoom factor/);
  });

  it("fits and centers the content while reserving the camera inset", () => {
    const viewport = { width: 1000, height: 700 };
    const content = { width: 1200, height: 800 };
    const camera = Camera.fit(viewport, content);
    const right = viewport.width - camera.x - content.width * camera.scale;
    const bottom = viewport.height - camera.y - content.height * camera.scale;
    expect(camera.x).toBeGreaterThanOrEqual(CAMERA_LIMITS.fitInset);
    expect(camera.y).toBeGreaterThanOrEqual(CAMERA_LIMITS.fitInset);
    expect(right).toBeCloseTo(camera.x);
    expect(bottom).toBeCloseTo(camera.y);
    expect(camera.scale).toBeLessThan(1);
    expect(Camera.fit(viewport, { width: 100, height: 100 }).scale).toBe(1);
  });

  it("fits paired boards below 20% without clipping either edge", () => {
    const camera = Camera.fit(narrowCanvas, pairedBoards);
    const right = narrowCanvas.width - camera.x - pairedBoards.width * camera.scale;
    const bottom = narrowCanvas.height - camera.y - pairedBoards.height * camera.scale;
    expect(camera.scale).toBeLessThan(0.2);
    expect(camera.scale).toBeGreaterThan(CAMERA_LIMITS.minimumScale);
    expect(camera.x).toBeCloseTo(CAMERA_LIMITS.fitInset);
    expect(right).toBeCloseTo(camera.x);
    expect(camera.y).toBeGreaterThanOrEqual(CAMERA_LIMITS.fitInset);
    expect(bottom).toBeCloseTo(camera.y);
  });

  it("pans and zooms around the cursor from a fitted scale below 20%", () => {
    const fitted = Camera.fit(narrowCanvas, pairedBoards);
    expect(fitted.pan({ x: 12, y: -8 })).toMatchObject({
      x: fitted.x + 12,
      y: fitted.y - 8,
      scale: fitted.scale,
    });
    const anchor = { x: 200, y: 90 };
    const world = {
      x: (anchor.x - fitted.x) / fitted.scale,
      y: (anchor.y - fitted.y) / fitted.scale,
    };
    for (const factor of [1 / CAMERA_LIMITS.zoomStep, CAMERA_LIMITS.zoomStep]) {
      const next = fitted.zoom(factor, anchor);
      expect(next.scale).toBeCloseTo(fitted.scale * factor);
      expect((anchor.x - next.x) / next.scale).toBeCloseTo(world.x);
      expect((anchor.y - next.y) / next.scale).toBeCloseTo(world.y);
    }
    expect(fitted.zoom(1e-6, anchor).scale).toBe(CAMERA_LIMITS.minimumScale);
  });

  it("falls back to a usable camera until positive layout bounds are available", () => {
    for (const width of [0, -1, NaN, Infinity]) {
      expect(Camera.fit({ width, height: 500 }, { width: 300, height: 200 })).toEqual(Camera.at());
      expect(Camera.fit({ width: 500, height: 300 }, { width, height: 200 })).toEqual(Camera.at());
    }
  });
});

describe("camera persistence boundary", () => {
  it("roundtrips the admitted camera without replacing it with a fitted view", () => {
    const camera = Camera.at(123.5, -420.25, 0.75);
    expect(Camera.restore(camera.serialize())).toEqual(camera);
  });

  it("roundtrips a fitted camera below 20%", () => {
    const fitted = Camera.fit(narrowCanvas, pairedBoards);
    expect(fitted.scale).toBeLessThan(0.2);
    expect(Camera.restore(fitted.serialize())).toEqual(fitted);
    saveCamera("business", fitted);
    expect(readCamera("business")).toEqual(fitted);
  });

  it.each([
    null,
    "",
    "not-json",
    "null",
    "[]",
    "42",
    '{"version":2,"x":0,"y":0,"scale":1}',
    '{"version":1,"x":"0","y":0,"scale":1}',
    '{"version":1,"x":0,"y":0}',
    '{"version":1,"x":1e309,"y":0,"scale":1}',
    JSON.stringify({ version: 1, x: CAMERA_LIMITS.maximumOffset + 1, y: 0, scale: 1 }),
    JSON.stringify({ version: 1, x: 0, y: -CAMERA_LIMITS.maximumOffset - 1, scale: 1 }),
    JSON.stringify({ version: 1, x: 0, y: 0, scale: CAMERA_LIMITS.minimumScale / 2 }),
    JSON.stringify({ version: 1, x: 0, y: 0, scale: CAMERA_LIMITS.maximumScale * 2 }),
    " ".repeat(CAMERA_LIMITS.storageBytes + 1),
  ])("rejects corrupt, incompatible, oversized or out-of-bounds stored data %#", (raw) => {
    expect(Camera.restore(raw)).toBeNull();
  });

  it("keeps the two panels' cameras independent", () => {
    const foundation = Camera.at(20, 30, 0.5);
    const business = Camera.at(-300, 50, 1.5);
    saveCamera("foundation", foundation);
    saveCamera("business", business);
    expect(readCamera("foundation")).toEqual(foundation);
    expect(readCamera("business")).toEqual(business);
  });

  it("tolerates unavailable storage without changing the camera", () => {
    const read = vi.spyOn(localStorage, "getItem").mockImplementation(() => {
      throw new DOMException("blocked", "SecurityError");
    });
    expect(readCamera("foundation")).toBeNull();
    expect(read).toHaveBeenCalledOnce();
  });

  it.each(["SecurityError", "QuotaExceededError"])("tolerates %s while saving", (name) => {
    const write = vi.spyOn(localStorage, "setItem").mockImplementation(() => {
      throw new DOMException("blocked", name);
    });
    expect(() => saveCamera("foundation", Camera.at())).not.toThrow();
    expect(write).toHaveBeenCalledOnce();
  });

  it("does not disguise an unexpected storage failure as success", () => {
    const failure = new Error("unexpected storage failure");
    vi.spyOn(localStorage, "getItem").mockImplementation(() => {
      throw failure;
    });
    vi.spyOn(localStorage, "setItem").mockImplementation(() => {
      throw failure;
    });
    expect(() => readCamera("foundation")).toThrow(failure);
    expect(() => saveCamera("foundation", Camera.at())).toThrow(failure);
  });
});
