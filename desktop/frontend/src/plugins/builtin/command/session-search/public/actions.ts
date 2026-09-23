import { useSessionSearchStore } from "../application/sessionSearchState";

export const SESSION_SEARCH_COMMAND = "chat.find";

export function openSessionSearch(): void {
  useSessionSearchStore.getState().show();
}
