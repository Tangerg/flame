import { type AgentRunView } from "@/features/agent/public/sessionView";
import { type RootRunSettlement } from "@/features/agent/application/run/rootAttention";
import { terminalSettlementStatus } from "@/features/agent/application/run/rootAttention";
export type RunAnnouncement = RootRunSettlement["status"] | "running" | null;

export function runAnnouncement(
  status: AgentRunView["status"] | "idle",
  outcome: AgentRunView["outcome"],
): RunAnnouncement {
  switch (status) {
    case "running":
      return "running";
    case "waiting":
      return "needsInput";
    case "finished":
      return terminalSettlementStatus(outcome);
    default:
      return null;
  }
}

const KEY: Record<Exclude<RunAnnouncement, null>, string> = {
  running: "announce.running",
  needsInput: "announce.needsInput",
  finished: "announce.finished",
  error: "announce.error",
  canceled: "announce.canceled",
};

export function runAnnouncementKey(announcement: RunAnnouncement): string | null {
  return announcement === null ? null : KEY[announcement];
}
