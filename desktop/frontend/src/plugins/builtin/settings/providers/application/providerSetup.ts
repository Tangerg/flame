/** The shape the setup check needs — an api key that is either saved or not. Kept
 *  narrow on purpose: this predicate must not grow into a second reading of the
 *  provider list. */
export interface ProviderCredentialSummary {
  configured: boolean;
}

/**
 * `undefined` is "still loading" and deliberately NOT "needs setup": the query resolves a
 * beat after mount, and answering true there flashes the setup prompt on every cold start
 * of a fully configured app.
 */
export function needsProviderSetup(providers: ProviderCredentialSummary[] | undefined): boolean {
  return providers !== undefined && !providers.some((provider) => provider.configured);
}
