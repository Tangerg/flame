import type { Scheme } from "@/lib/appearance";
import { createSingletonPort } from "@/lib/ports/singletonPort";

/**
 * A port because resolving a theme id to its scheme is an `application/` rule, and a
 * `window.matchMedia` call there would make that rule need a browser to be exercised at all.
 */
interface SystemAppearancePort {
  scheme(): Scheme;
}

const port = createSingletonPort<SystemAppearancePort>("System appearance port is not configured");

export const configureSystemAppearancePort = port.configure;
export const systemAppearance = port.get;
