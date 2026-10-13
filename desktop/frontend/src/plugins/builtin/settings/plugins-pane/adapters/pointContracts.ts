type PointMetadata = {
  reference: string;
  composition: "collection" | "selection" | "decoration";
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
