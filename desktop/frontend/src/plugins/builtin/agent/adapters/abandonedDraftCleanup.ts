import { navigator } from "@/lib/navigation";
import { discardAbandonedDraft } from "../application/session/discardAbandonedDraft";

/**
 * One subscriber rather than a call at each selection site: sessions are selected from the
 * sidebar, the ⌘K palette, deeplinks and recovery, and a rule about what the user left
 * behind belongs to the transition, not to each caller that happens to cause one.
 */
export function installAbandonedDraftCleanup(): () => void {
  return navigator().subscribe((next, previous) => {
    if (previous.session === next.session) return;
    discardAbandonedDraft(previous.session);
  });
}
