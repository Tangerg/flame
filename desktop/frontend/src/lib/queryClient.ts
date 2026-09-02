import { QueryClient } from "@tanstack/react-query";
import type { RetirableTaskCohort } from "./taskQueue";

// Single QueryClient for the app. Defaults are conservative: no auto-retry
// on failure (the user can manually refetch via re-render / staleness), and
// a 1-minute default stale window for resources we don't override.
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 60_000,
    },
  },
});

/** An accepted command already holds the authoritative resource, so a failed cache repair is
 *  not a command failure: Runtime events and the next read remain the repair path. Nothing
 *  propagates, including the cohort's retirement — the caller re-asserts afterwards anyway,
 *  over a window that also covers a retirement arriving after the repair settled. */
export async function repairCachedProjection(
  cohort: RetirableTaskCohort,
  keys: readonly string[],
): Promise<void> {
  try {
    await Promise.all(
      keys.map((key) => cohort.settle(queryClient.invalidateQueries({ queryKey: [key] }))),
    );
  } catch {
    // Deliberately swallowed; see above.
  }
}
