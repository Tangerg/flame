import type { ExtensionPoint as ContractToken } from "dougong";
import type { Contribution } from "../contracts";

export type ExtensionKeying = "single" | "multi";

export interface ExtensionPoint<T> {
  readonly id: string;
  readonly keying: ExtensionKeying;
  readonly token: ContractToken<Contribution<T>>;
  readonly keyOf?: (item: T) => string;
  readonly normalizeKey?: (key: string) => string;
}

export interface ExtensionContributionOptions {
  id?: string;
  key?: string;
  order?: number;
}
