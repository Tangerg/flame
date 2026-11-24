export const OPTIMISTIC_USER_MESSAGE_PREFIX = "local-";

const OPTIMISTIC_STEER_MESSAGE_PREFIX = `${OPTIMISTIC_USER_MESSAGE_PREFIX}steer-`;

// A steer shows under its committed Item once known, else under its command key.
export function steerMessageId(idempotencyKey: string, userItemId?: string | null): string {
  return userItemId ?? `${OPTIMISTIC_STEER_MESSAGE_PREFIX}${idempotencyKey}`;
}

// A retired steer stays visible beside its committed Item, never as it.
export function retiredSteerMessageId(userItemId: string): string {
  return `retired-steer:${userItemId}`;
}
