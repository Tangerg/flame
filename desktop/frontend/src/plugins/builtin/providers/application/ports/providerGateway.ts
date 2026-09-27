import type { ProviderConfiguration, ProviderRole } from "../providerModels";

export interface ProviderUpdate {
  provider: string;
  apiKey?: ProviderSettingChange;
  baseUrl?: ProviderSettingChange;
}

export type ProviderSettingChange = { type: "set"; value: string } | { type: "clear" };

export interface ProviderTestOutcome {
  ok: boolean;
  error?: string;
}

export interface ProviderGateway {
  updateProvider(input: ProviderUpdate): Promise<ProviderConfiguration>;
  setUtilityRole(role: ProviderRole): Promise<ProviderRole>;
  setEmbeddingRole(role: ProviderRole): Promise<ProviderRole>;
  testProvider(provider: string): Promise<ProviderTestOutcome>;
  errorMessage(error: unknown): string | undefined;
}
