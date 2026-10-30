import { QueryClient, type QueryFilters } from "@tanstack/react-query";
export const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      retry: 1,
      refetchOnWindowFocus: false,
      staleTime: 60_000,
    },
  },
});

export function replaceCachedRead(options?: QueryFilters): Promise<void> {
  void queryClient.cancelQueries(options);
  return queryClient.invalidateQueries(options);
}
