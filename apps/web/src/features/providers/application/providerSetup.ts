export interface ProviderCredentialSummary {
  configured: boolean;
}

export function needsProviderSetup(providers: ProviderCredentialSummary[] | undefined): boolean {
  return providers !== undefined && !providers.some((provider) => provider.configured);
}
