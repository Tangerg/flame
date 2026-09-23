import { useCallback, useRef, useState } from "react";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError, type NotifySource } from "./notifications";

export interface CommandActionConfig {
  wasRetired: (error: unknown) => boolean;
  fallback: string;
  source?: NotifySource;
}

export interface CommandAction {
  busy: boolean;
  run: (command: () => Promise<unknown>) => void;
}

export function useCommandAction({
  wasRetired,
  fallback,
  source,
}: CommandActionConfig): CommandAction {
  const inFlight = useRef(false);
  const [busy, setBusy] = useState(false);

  const run = useCallback(
    (command: () => Promise<unknown>) => {
      if (inFlight.current) return;
      inFlight.current = true;
      setBusy(true);
      command()
        .catch((error: unknown) => {
          if (wasRetired(error)) return;
          notifyError(rpcErrorText(error) ?? fallback, source ? { source } : undefined);
        })
        .finally(() => {
          inFlight.current = false;
          setBusy(false);
        });
    },
    [fallback, source, wasRetired],
  );

  return { busy, run };
}
