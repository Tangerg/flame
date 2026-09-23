import { navigator } from "@/lib/navigation";
import { discardAbandonedDraft } from "../application/session/discardAbandonedDraft";

export function installAbandonedDraftCleanup(): () => void {
  return navigator().subscribe((next, previous) => {
    if (previous.session === next.session) return;
    discardAbandonedDraft(previous.session);
  });
}
