import { useCallback, useMemo } from "react";
import {
  failureText,
  useContextState,
  type ComponentContext,
} from "@flame/component-sdk";
import { stringifyJSON } from "@frame/sdk/json";
import type { MethodInputs, MethodOutputs } from "./methods.generated";
import { METHODS } from "../contracts/methods";
import type { DeliverableOperations } from "./operations";

export type ViewMethod =
  | typeof METHODS.catalog
  | typeof METHODS.list
  | typeof METHODS.read
  | typeof METHODS.history
  | typeof METHODS.uploads;
type Projection<K extends ViewMethod> = {
  input: MethodInputs[K];
  value?: MethodOutputs[K];
  loading: boolean;
  error?: string;
};
type Projections = { [K in ViewMethod]?: Projection<K> };

export function useProjections(
  context: ComponentContext,
  operations: DeliverableOperations,
  signal: AbortSignal,
) {
  const requests = useMemo(
    () => new Map<ViewMethod, AbortController>(),
    [context],
  );
  const [projections, setProjections] = useContextState<Projections>(context);

  const query = useCallback(
    async <K extends ViewMethod>(
      method: K,
      input: MethodInputs[K],
    ): Promise<MethodOutputs[K] | undefined> => {
      if (signal.aborted) return;
      requests.get(method)?.abort();
      const request = new AbortController();
      requests.set(method, request);
      const lifetime = AbortSignal.any([signal, request.signal]);
      setProjections((previous) => {
        const existing = previous ?? {};
        const prior = existing[method] as Projection<K> | undefined;
        const retain =
          method === METHODS.list ||
          method === METHODS.catalog ||
          method === METHODS.uploads ||
          (method === METHODS.history &&
            prior &&
            "id" in prior.input &&
            "id" in input &&
            prior.input.id === input.id) ||
          (prior && stringifyJSON(prior.input) === stringifyJSON(input));
        return {
          ...existing,
          [method]: {
            input: retain && prior?.value ? prior.input : input,
            value: retain ? prior?.value : undefined,
            loading: true,
          },
        };
      });
      try {
        const value = await operations.query(method, input, lifetime);
        if (lifetime.aborted) return;
        setProjections((previous) => ({
          ...previous,
          [method]: { input, value, loading: false },
        }));
        return value;
      } catch (error) {
        if (!lifetime.aborted)
          setProjections((previous) => {
            const existing = previous ?? {};
            return {
              ...existing,
              [method]: {
                ...existing[method],
                input: existing[method]?.input ?? input,
                loading: false,
                error: failureText(error),
              },
            };
          });
      }
    },
    [operations, requests, setProjections, signal],
  );
  const abort = useCallback(
    (method: ViewMethod) => requests.get(method)?.abort(),
    [requests],
  );

  return { values: projections ?? {}, query, abort };
}
