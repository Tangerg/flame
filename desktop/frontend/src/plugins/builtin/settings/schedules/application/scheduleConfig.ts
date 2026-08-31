export interface ScheduleConfig {
  id: string;
  title: string;
  instructions: string;
  cwd?: string;
  cron: string;
  enabled: boolean;
  provider?: string;
  model?: string;
  createdAt?: string;
  nextRunAt?: string;
  lastRunAt?: string;
  revision: number;
}

export type { ScheduleDraft as ScheduleConfigInput } from "./scheduleDraft";

export interface ScheduledRunIdentity {
  sessionId: string;
  runId: string;
}
