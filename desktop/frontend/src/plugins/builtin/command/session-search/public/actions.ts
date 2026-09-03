// Published actions for opening the session finder from another bounded context.
import { useSessionSearchStore } from "../application/sessionSearchState";

/** The finder's command id, so a surface that shows its key reads the key off the command
 *  rather than spelling it a second time. */
export const SESSION_SEARCH_COMMAND = "chat.find";

export function openSessionSearch(): void {
  useSessionSearchStore.getState().show();
}
