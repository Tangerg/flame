import { QueryClient } from "@tanstack/react-query";
import type { RetirableTaskCohort } from "./taskQueue";

export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 60_000,
    },
  },
});

export async function repairCachedProjection(
  cohort: RetirableTaskCohort,
  keys: readonly string[],
): Promise<void> {
  try {
    await Promise.all(
      keys.map((key) => cohort.settle(queryClient.invalidateQueries({ queryKey: [key] }))),
    );
  } catch {}
}

export function replaceCachedRead(options?: {
  queryKey: readonly unknown[];
  exact?: boolean;
}): Promise<void> {
  void queryClient.cancelQueries(options);
  return queryClient.invalidateQueries(options);
}
