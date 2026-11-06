export interface Point {
  readonly x: number;
  readonly y: number;
}

export interface Size {
  readonly width: number;
  readonly height: number;
}

export const CAMERA_LIMITS = {
  minimumScale: 0.2,
  maximumScale: 2,
  maximumOffset: 100_000,
  fitInset: 32,
  keyboardStep: 48,
  zoomStep: 1.2,
  wheelSensitivity: 0.002,
  storageBytes: 256,
} as const;

const CAMERA_VERSION = 1;
const CAMERA_STORAGE_PREFIX = "flame.component-lab.camera";
const bound = (value: number, min: number, max: number) => Math.min(max, Math.max(min, value));

export class Camera {
  private constructor(
    readonly x: number,
    readonly y: number,
    readonly scale: number,
  ) {}

  static at(x = 0, y = 0, scale = 1): Camera {
    if (![x, y, scale].every(Number.isFinite)) throw new Error("Camera values must be finite");
    return new Camera(
      bound(x, -CAMERA_LIMITS.maximumOffset, CAMERA_LIMITS.maximumOffset),
      bound(y, -CAMERA_LIMITS.maximumOffset, CAMERA_LIMITS.maximumOffset),
      bound(scale, CAMERA_LIMITS.minimumScale, CAMERA_LIMITS.maximumScale),
    );
  }

  pan(delta: Point): Camera {
    return Camera.at(this.x + delta.x, this.y + delta.y, this.scale);
  }

  zoom(factor: number, anchor: Point): Camera {
    if (!Number.isFinite(factor) || factor <= 0) throw new Error("Invalid zoom factor");
    const scale = bound(
      this.scale * factor,
      CAMERA_LIMITS.minimumScale,
      CAMERA_LIMITS.maximumScale,
    );
    const ratio = scale / this.scale;
    return Camera.at(
      anchor.x - (anchor.x - this.x) * ratio,
      anchor.y - (anchor.y - this.y) * ratio,
      scale,
    );
  }

  static fit(viewport: Size, content: Size): Camera {
    if (
      ![viewport.width, viewport.height, content.width, content.height].every(
        (value) => Number.isFinite(value) && value > 0,
      )
    ) {
      return Camera.at();
    }
    const inset = CAMERA_LIMITS.fitInset;
    const scale = bound(
      Math.min(
        (viewport.width - inset * 2) / content.width,
        (viewport.height - inset * 2) / content.height,
        1,
      ),
      CAMERA_LIMITS.minimumScale,
      CAMERA_LIMITS.maximumScale,
    );
    return Camera.at(
      (viewport.width - content.width * scale) / 2,
      (viewport.height - content.height * scale) / 2,
      scale,
    );
  }

  serialize(): string {
    return JSON.stringify({ version: CAMERA_VERSION, x: this.x, y: this.y, scale: this.scale });
  }

  static restore(raw: string | null): Camera | null {
    if (!raw || raw.length > CAMERA_LIMITS.storageBytes) return null;
    try {
      const value: unknown = JSON.parse(raw);
      if (typeof value !== "object" || value === null) return null;
      const record = value as Record<string, unknown>;
      if (
        record.version !== CAMERA_VERSION ||
        typeof record.x !== "number" ||
        typeof record.y !== "number" ||
        typeof record.scale !== "number"
      )
        return null;
      if (![record.x, record.y, record.scale].every(Number.isFinite)) return null;
      if (
        record.scale < CAMERA_LIMITS.minimumScale ||
        record.scale > CAMERA_LIMITS.maximumScale ||
        Math.abs(record.x) > CAMERA_LIMITS.maximumOffset ||
        Math.abs(record.y) > CAMERA_LIMITS.maximumOffset
      )
        return null;
      return Camera.at(record.x, record.y, record.scale);
    } catch (error) {
      if (error instanceof SyntaxError) return null;
      throw error;
    }
  }
}

export function readCamera(panel: string): Camera | null {
  try {
    return Camera.restore(localStorage.getItem(`${CAMERA_STORAGE_PREFIX}.${panel}`));
  } catch (error) {
    if (error instanceof DOMException && error.name === "SecurityError") return null;
    throw error;
  }
}

export function saveCamera(panel: string, camera: Camera): void {
  try {
    localStorage.setItem(`${CAMERA_STORAGE_PREFIX}.${panel}`, camera.serialize());
  } catch (error) {
    if (
      error instanceof DOMException &&
      ["SecurityError", "QuotaExceededError"].includes(error.name)
    )
      return;
    throw error;
  }
}
