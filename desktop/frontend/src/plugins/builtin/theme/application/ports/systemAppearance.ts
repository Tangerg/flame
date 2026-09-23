import type { Scheme } from "@/lib/appearance";
import { createSingletonPort } from "@/lib/ports/singletonPort";

interface SystemAppearancePort {
  scheme(): Scheme;
}

const port = createSingletonPort<SystemAppearancePort>("System appearance port is not configured");

export const configureSystemAppearancePort = port.configure;
export const systemAppearance = port.get;
