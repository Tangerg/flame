import { useCallback, useRef, useState } from "react";
import { rpcErrorText } from "@/lib/rpcErrors";
import { notifyError, type NotifySource } from "./notifications";

export interface CommandActionConfig {
  /** The generation predicate this owner publishes. A command the owner retired settles
   *  nowhere the user can see, so it is not a failure to report. */
  wasRetired: (error: unknown) => boolean;
  /** Already localized: what to say when the Runtime gave no message of its own. */
  fallback: string;
  source?: NotifySource;
}

export interface CommandAction {
  busy: boolean;
  run: (command: () => Promise<unknown>) => void;
}

/**
 * One user-triggered Runtime command at a time, with the fact that it is running visible on
 * the control that started it.
 *
 * The re-entry guard is a ref rather than the state: `busy` reaches the DOM a render later,
 * so a second click landing inside that gap would otherwise fire the command twice — which,
 * on a delete or a run-now, is two commands and not one wasted render.
 */
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
