import { createSingletonPort } from "@/lib/ports/singletonPort";

/**
 * A port because the candidate list is ours — a curated cross-platform set — while the probe
 * is the browser's, and keeping it behind here leaves that list testable without a DOM.
 */
interface FontAvailabilityPort {
  isAvailable(family: string): boolean;
}

const port = createSingletonPort<FontAvailabilityPort>("Font availability port is not configured");

export const configureFontAvailabilityPort = port.configure;
export const fontAvailability = port.get;
