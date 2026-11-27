import { bindOperations, type ComponentContext } from "@flame/component-sdk";
import {
  CAPABILITY_JSON,
  type MethodInputs,
  type MethodOutputs,
} from "./methods.generated";

export function bindDeliverableOperations(
  context: Pick<ComponentContext, "binding" | "runtime" | "signal" | "track">,
) {
  if (!context.binding)
    throw new Error(
      "deliverables component requires its callable contribution",
    );
  return bindOperations<MethodInputs, MethodOutputs>(context, CAPABILITY_JSON, {
    scope: "installation",
    reference: context.binding.installationId,
  });
}

export type DeliverableOperations = ReturnType<
  typeof bindDeliverableOperations
>;
