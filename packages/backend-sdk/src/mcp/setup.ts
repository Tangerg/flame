import { Lifetime, type Cleanup, type Release } from "@frame/sdk";
import { Capability, exactFields, validName } from "./contract.ts";
import { type CallbackRef, type Invocation } from "./callback.ts";

export interface SetupBinding {
  readonly installationId: string;
  readonly digest: string;
  readonly contribution: string;
  readonly contract: import("./contract.ts").CapabilityDescriptor;
}

export interface SetupContribution {
  readonly id: string;
  readonly export: string;
  readonly background?: Invocation["target"];
  readonly requires: ReadonlyArray<{
    readonly id: string;
    readonly contract: import("./contract.ts").CapabilityDescriptor;
    readonly optional: boolean;
    readonly bindings: readonly SetupBinding[];
  }>;
}

export function validateRequirements(
  dependencies: SetupContribution["requires"],
): void {
  const requirements = new Set<string>();
  for (const dependency of dependencies) {
    exactFields(dependency, ["id", "contract", "optional", "bindings"]);
    if (
      typeof dependency.id !== "string" ||
      !validName(dependency.id) ||
      requirements.has(dependency.id) ||
      typeof dependency.optional !== "boolean" ||
      !Array.isArray(dependency.bindings)
    )
      throw new Error("plugin: invalid setup dependency");
    requirements.add(dependency.id);
    const contract = new Capability(dependency.contract);
    if (
      (!dependency.optional && !dependency.bindings.length) ||
      (contract.descriptor.composition === "selection" &&
        dependency.bindings.length > 1)
    )
      throw new Error("plugin: invalid dependency cardinality");
    for (const binding of dependency.bindings)
      validateSetupBinding(binding, contract);
  }
}

function validateSetupBinding(
  binding: SetupBinding,
  contract: Capability,
): void {
  exactFields(binding, [
    "installationId",
    "digest",
    "contribution",
    "contract",
  ]);
  if (
    typeof binding.installationId !== "string" ||
    !validName(binding.installationId) ||
    typeof binding.contribution !== "string" ||
    !validName(binding.contribution) ||
    typeof binding.digest !== "string" ||
    !/^[a-f0-9]{64}$/.test(binding.digest) ||
    new Capability(binding.contract).identity !== contract.identity
  )
    throw new Error("plugin: invalid captured dependency binding");
}

export interface ActivationResource {
  close(): void | Promise<void>;
  start?(): void | Promise<void>;
}

export interface SetupContext {
  readonly signal: AbortSignal;
  readonly contributions: readonly SetupContribution[];
  readonly callback?: CallbackRef;
  readonly lifetime: Lifetime;
  defer(cleanup: () => void | Release | Promise<void | Release>): Cleanup;
}

export type SetupHandler = (
  context: SetupContext,
) => void | ActivationResource | Promise<void | ActivationResource>;

export function assertBackground(background: unknown): void {
  exactFields(background, ["scope", "reference"]);
  if (
    background.scope !== "installation" ||
    typeof background.reference !== "string" ||
    !validName(background.reference)
  )
    throw new Error("plugin: background must identify its installation");
}
