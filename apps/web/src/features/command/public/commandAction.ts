import { useNotifications } from "@/features/notifications/public/notifier";
import { useCallback, useRef, useState } from "react";
import { rpcErrorText } from "@/lib/rpcErrors";
import { type NotifySource } from "@/features/notifications/public/notifications";
export interface CommandActionConfig {
  wasRetired?: (error: unknown) => boolean;
  fallback: string;
  source?: NotifySource;
  onError?: (message: string, error: unknown) => void;
}

export interface CommandAction {
  busy: boolean;
  run: (command: () => Promise<unknown>) => void;
}

export function useCommandAction({
  wasRetired,
  fallback,
  source,
  onError,
}: CommandActionConfig): CommandAction {
  const { messages } = useNotifications();
  const inFlight = useRef(false);
  const [busy, setBusy] = useState(false);

  const run = useCallback(
    (command: () => Promise<unknown>) => {
      if (inFlight.current) return;
      inFlight.current = true;
      setBusy(true);
      command()
        .catch((error: unknown) => {
          if (wasRetired?.(error)) return;
          const message = rpcErrorText(error) ?? fallback;
          if (onError) onError(message, error);
          else messages.notify("error", message, source ? { source } : undefined);
        })
        .finally(() => {
          inFlight.current = false;
          setBusy(false);
        });
    },
    [fallback, onError, source, wasRetired, messages],
  );

  return { busy, run };
}
