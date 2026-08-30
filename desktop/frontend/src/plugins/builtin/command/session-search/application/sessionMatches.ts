// Jumping to a session by NAME, and nothing else: commands have buttons and shortcuts, and
// the dock's own picker groups panels by scope. The sidebar lists every session but cannot
// filter, so finding one by name means scrolling.

import type { AgentSessionSummary } from "@/plugins/builtin/agent/public/session";

/** Enough to fill the panel twice over; past that a person narrows the query
 *  instead of scrolling, and the DOM stays bounded either way. */
const DEFAULT_LIMIT = 20;

function byNewest(a: AgentSessionSummary, b: AgentSessionSummary): number {
  if (a.time === b.time) return 0;
  return a.time < b.time ? 1 : -1;
}

/**
 * An empty query answers with the most recent rather than with nothing: this surface exists
 * to go somewhere, so a blank list would make the common case — back to what I was just
 * doing — the slowest one. The command palette differs because it has commands to show.
 */
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
