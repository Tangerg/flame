import { createSingletonPort } from "@/lib/ports/singletonPort";

interface FontAvailabilityPort {
  isAvailable(family: string): boolean;
  hasTabularFigures(family: string): boolean;
}

const port = createSingletonPort<FontAvailabilityPort>("Font availability port is not configured");

export const configureFontAvailabilityPort = port.configure;
export const fontAvailability = port.get;
