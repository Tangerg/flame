import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";

const DEFAULT_LIMIT = 20;

function byNewest(a: AgentSessionSummary, b: AgentSessionSummary): number {
  if (a.time === b.time) return 0;
  return a.time < b.time ? 1 : -1;
}

export function matchSessions(
  sessions: readonly AgentSessionSummary[],
  query: string,
  limit = DEFAULT_LIMIT,
): AgentSessionSummary[] {
  const needle = query.trim().toLowerCase();
  const matched =
    needle === ""
      ? [...sessions]
      : sessions.filter((session) => session.title.toLowerCase().includes(needle));
  return matched.sort(byNewest).slice(0, limit);
}
