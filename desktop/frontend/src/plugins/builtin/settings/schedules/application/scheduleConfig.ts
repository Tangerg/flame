export interface ScheduleConfig {
  id: string;
  title: string;
  instructions: string;
  cwd?: string;
  cron: string;
  enabled: boolean;
  provider?: string;
  model?: string;
  reasoningEffort?: string;
  createdAt?: string;
  nextRunAt?: string;
  lastRunAt?: string;
  revision: number;
}

export interface ScheduleModelSelection {
  provider: string;
  model: string;
  reasoningEffort?: string;
}

export interface ScheduleConfigInput {
  title: string;
  instructions: string;
  cron: string;
  cwd: string;
  modelSelection?: ScheduleModelSelection | null;
}

export interface ScheduledRunIdentity {
  sessionId: string;
  runId: string;
}
