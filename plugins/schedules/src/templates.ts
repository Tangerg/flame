export const templates = [
  {
    id: "hourly",
    title: "Hourly progress check",
    cron: "0 * * * *",
    instructions:
      "Inspect the current workspace and summarize recent progress, outstanding work and any blockers.",
  },
  {
    id: "daily",
    title: "Daily review",
    cron: "0 9 * * *",
    instructions:
      "Review recent changes in the current workspace and summarize risks, test results and next steps.",
  },
  {
    id: "weekdays",
    title: "Weekday planning",
    cron: "0 9 * * 1-5",
    instructions:
      "Review the current workspace and propose the most useful next steps for today.",
  },
  {
    id: "weekly",
    title: "Weekly maintenance review",
    cron: "0 9 * * 1",
    instructions:
      "Review the current workspace for maintenance work and summarize concrete improvements and their priority.",
  },
] as const;
