import { getContainer } from "@/main/container";
import { configureUsageGateway, type UsageGateway } from "../application/ports/usageGateway";

const gateway: UsageGateway = {
  loadSummary(period, signal) {
    const sinceDays = period.recentDays();
    return getContainer()
      .client()
      .usage.summary(sinceDays === undefined ? {} : { sinceDays }, signal);
  },
};

export function installUsageGateway(): () => void {
  return configureUsageGateway(gateway);
}
