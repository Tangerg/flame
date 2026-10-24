import { useState } from "react";
import { equalJSON, isJSONNumber, parseJSON } from "@flame/runtime-client/json";
export interface ApprovalArgsEditor {
  editing: boolean;
  setEditing: (editing: boolean) => void;
  argsText: string;
  setArgsText: (text: string) => void;
  invalid: boolean;
  commit: () => Record<string, unknown> | undefined | null;
}

export function commitApprovalArgs(
  originalArgs: string,
  argsText: string,
): Record<string, unknown> | undefined | null {
  const parsed = parseArgs(argsText);
  if (!parsed.ok) return null;
  const original = parseArgs(originalArgs);
  if (!original.ok) return parsed.value;
  return equalJSON(original.value, parsed.value) ? undefined : parsed.value;
}

export function useApprovalArgsEditor({
  originalArgs,
}: {
  originalArgs: string;
}): ApprovalArgsEditor {
  const [editing, setEditing] = useState(false);
  const [argsText, setArgsTextState] = useState(originalArgs);
  const [invalid, setInvalid] = useState(false);

  const setArgsText = (text: string) => {
    setArgsTextState(text);
    setInvalid(false);
  };

  const commit = (): Record<string, unknown> | undefined | null => {
    const result = commitApprovalArgs(originalArgs, argsText);
    if (result === null) setInvalid(true);
    return result;
  };

  return { editing, setEditing, argsText, setArgsText, invalid, commit };
}

function parseArgs(value: string): { ok: true; value: Record<string, unknown> } | { ok: false } {
  try {
    const parsed = parseJSON(value);
    if (!isRecord(parsed)) return { ok: false };
    return { ok: true, value: parsed };
  } catch {
    return { ok: false };
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return (
    typeof value === "object" && value !== null && !Array.isArray(value) && !isJSONNumber(value)
  );
}
