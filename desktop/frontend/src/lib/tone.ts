/** What a state MEANS, not what colour it is. Application layers emit this vocabulary; the
 *  Badge picks the fill and ink. */
export type Tone = "neutral" | "accent" | "success" | "warning" | "negative" | "info";

/** The same idea for a live dot, which reports a lifecycle rather than a severity — so it has
 *  its own words (`running`, `waiting`) and lives here for the same reason `Tone` does: an
 *  application layer names the meaning and may not reach the atom that renders it. */
export type DotTone = "idle" | "running" | "waiting" | "ok" | "err";
