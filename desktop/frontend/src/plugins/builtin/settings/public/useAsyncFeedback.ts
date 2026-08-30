import { useLayoutEffect, useRef, useState } from "react";

export type AsyncFeedback =
  { state: "idle" | "busy" } | { state: "ok" } | { state: "error"; reason: string };

/**
 * Drive an inline async-operation indicator with stale-result de-racing.
 *
 * An exact lease guards every {@link run}: a result whose lease is no longer
 * current — a newer run started, or `reset` bumped it — is dropped, so a slow
 * operation cannot overwrite feedback for a newer intent or replacement resource.
 * An optional material generation retires both completed feedback and in-flight
 * results without remounting or discarding the caller's draft fields.
 * `reset` invalidates any in-flight run and clears the readout; `fail` sets an
 * error directly (for flows, like delete, that don't need the de-race guard).
 */
export function useAsyncFeedback(materialGeneration?: unknown) {
  const generation = useRef(materialGeneration);
  const lease = useRef<object>({});
  const [material, setMaterial] = useState<{ generation: unknown; feedback: AsyncFeedback }>(
    () => ({
      generation: materialGeneration,
      feedback: { state: "idle" },
    }),
  );
  useLayoutEffect(() => {
    if (Object.is(generation.current, materialGeneration)) return;
    generation.current = materialGeneration;
    lease.current = {};
  }, [materialGeneration]);
  const feedback = Object.is(material.generation, materialGeneration)
    ? material.feedback
    : ({ state: "idle" } satisfies AsyncFeedback);

  const publish = (next: AsyncFeedback) => {
    setMaterial({ generation: generation.current, feedback: next });
  };

  const reset = () => {
    lease.current = {};
    publish({ state: "idle" });
  };

  const fail = (reason: string) => publish({ state: "error", reason });

  const run = async (
    op: () => Promise<{ ok: boolean; error?: string }>,
    fallback: string,
    ignoreError?: (error: unknown) => boolean,
  ) => {
    const admittedLease = (lease.current = {});
    const admittedGeneration = generation.current;
    publish({ state: "busy" });
    try {
      const r = await op();
      if (lease.current !== admittedLease || !Object.is(generation.current, admittedGeneration))
        return;
      publish(r.ok ? { state: "ok" } : { state: "error", reason: r.error ?? fallback });
    } catch (err) {
      if (lease.current !== admittedLease || !Object.is(generation.current, admittedGeneration))
        return;
      if (ignoreError?.(err)) {
        publish({ state: "idle" });
        return;
      }
      fail(err instanceof Error ? err.message : fallback);
    }
  };

  return { feedback, reset, fail, run };
}
