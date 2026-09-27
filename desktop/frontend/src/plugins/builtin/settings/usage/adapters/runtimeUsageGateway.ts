import type { FlameClient } from "@flame/runtime-contract/client";
import { configureUsageGateway } from "../application/ports/usageGateway";

export function installUsageGateway(runtimeClient: () => FlameClient): () => void {
  return configureUsageGateway({
    loadSummary(period, signal) {
      const sinceDays = period.recentDays();
      return runtimeClient().usage.summary(sinceDays === undefined ? {} : { sinceDays }, signal);
    },
  });
}
