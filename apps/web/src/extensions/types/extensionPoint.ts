import { type ExtensionPoint as ContractToken } from "dougong";
import { type PublishedContribution } from "@/extensions/contracts";
export type ExtensionKeying = "single" | "multi";

export interface ExtensionPoint<T> {
  readonly id: string;
  readonly keying: ExtensionKeying;
  readonly token: ContractToken<PublishedContribution<T>>;
  readonly keyOf?: (item: T) => string;
  readonly normalizeKey?: (key: string) => string;
}

export interface ExtensionContributionOptions {
  readonly id?: string;
  readonly key?: string;
}
