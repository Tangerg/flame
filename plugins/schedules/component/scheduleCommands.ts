import { bindOperations } from "@flame/component-sdk";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "../package/io.github.tangerg.flame/contracts/methods.generated";
export function bindScheduleOperations(
  context: Parameters<typeof bindOperations>[0],
) {
  if (!context.binding)
    throw new Error("Schedules requires its own callable contribution");
  const operations = bindOperations<MethodInputs, MethodOutputs>(
    context,
    CAPABILITY_JSON,
    { scope: "installation", reference: context.binding.installationId },
  );
  return {
    query: operations.query,
    ...operations.authoring("flame.schedules/authoring"),
  };
}
