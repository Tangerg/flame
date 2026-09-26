import { navigator } from "@/lib/navigation";
import { discardAbandonedDraft } from "../application/session/discardAbandonedDraft";

export function installAbandonedDraftCleanup(currentScope: () => object): () => void {
  let selectionScope = currentScope();
  return navigator().subscribe((next, previous) => {
    if (previous.session === next.session) return;
    const previousScope = selectionScope;
    selectionScope = currentScope();
    if (previousScope !== selectionScope) return;
    discardAbandonedDraft(previous.session);
  });
}
