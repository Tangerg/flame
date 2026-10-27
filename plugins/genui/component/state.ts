import { useCallback } from "react";
import { useGetFieldValue } from "@openuidev/react-lang";
import { stateKey } from "../contracts/document";
import { useGenUIInteraction } from "./interaction";

export function useLocalValue<Value>(field: string) {
  const read = useGetFieldValue();
  const { update, readInitial } = useGenUIInteraction();
  const observed = read(undefined, stateKey(field));
  const value = (
    observed === undefined ? readInitial(field) : observed
  ) as Value;
  const setValue = useCallback(
    (next: Value) => update(field, next),
    [update, field],
  );
  return [value, setValue] as const;
}
