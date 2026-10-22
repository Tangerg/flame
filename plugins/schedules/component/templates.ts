export const templates = [
  { id: "hourly", cron: "0 * * * *" },
  { id: "daily", cron: "0 9 * * *" },
  { id: "weekdays", cron: "0 9 * * 1-5" },
  { id: "weekly", cron: "0 9 * * 1" },
] as const;
