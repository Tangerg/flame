import { INSTALLATION_SCOPE, type PluginComposition } from "@flame/runtime-client";
type PointMetadata = {
  reference: string;
  composition: PluginComposition;
  scope: string;
  ordering: "canonicalKeys" | "explicitList";
  optional: boolean;
};

// This is a display projection; requests retain the admitted contract string.
export function pointMetadata(contract: string): PointMetadata {
  const { reference, composition, scope, ordering, optional } = JSON.parse(
    contract,
  ) as PointMetadata;
  return { reference, composition, scope, ordering, optional };
}

// An installation-scoped point addresses the installation that provides it.
export function defaultPointReference(metadata: PointMetadata, installationId: string): string {
  return metadata.scope === INSTALLATION_SCOPE ? installationId : "";
}
