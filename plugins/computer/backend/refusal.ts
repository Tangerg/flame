import { Refusal } from "@flame/backend-sdk/mcp";

// Every category this backend reports to the model and its callers. A code is a
// stable branch point, so all raise sites share this one closed spelling.
export type RefusalCode =
  | "computer_closed"
  | "computer_unavailable"
  | "engine_unavailable"
  | "engine_unconfigured"
  | "human_required"
  | "input_capacity"
  | "invalid_arguments"
  | "invalid_lease"
  | "invalid_permission"
  | "invalid_receipt_page"
  | "invocation_required"
  | "lease_expired"
  | "operation_not_found"
  | "operation_retired"
  | "permission_retired"
  | "receipt_capacity"
  | "receipt_not_found"
  | "session_active"
  | "session_replaced"
  | "session_required"
  | "worker_replaced";

export function refusal(code: RefusalCode, message: string): Refusal {
  return new Refusal(code, message);
}
