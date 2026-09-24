import type { ProviderSettingChange, ProviderUpdate } from "./ports/providerGateway";

export class ProviderCredentialsDraft {
  private constructor(
    readonly apiKey: string,
    readonly baseUrl: string,
  ) {}

  static initial(provider: { baseUrl?: string }): ProviderCredentialsDraft {
    return new ProviderCredentialsDraft("", provider.baseUrl ?? "");
  }

  withAPIKey(apiKey: string): ProviderCredentialsDraft {
    return new ProviderCredentialsDraft(apiKey, this.baseUrl);
  }

  withBaseURL(baseUrl: string): ProviderCredentialsDraft {
    return new ProviderCredentialsDraft(this.apiKey, baseUrl);
  }

  settle(
    submitted: ProviderCredentialsDraft,
    saved: { baseUrl?: string },
  ): ProviderCredentialsDraft {
    return new ProviderCredentialsDraft(
      this.apiKey === submitted.apiKey ? "" : this.apiKey,
      this.baseUrl === submitted.baseUrl ? (saved.baseUrl ?? "") : this.baseUrl,
    );
  }

  dirty(provider: { baseUrl?: string }): boolean {
    return this.apiKey.trim() !== "" || this.baseUrl !== (provider.baseUrl ?? "");
  }

  valid(provider: { requiresBaseUrl?: boolean }): boolean {
    return !provider.requiresBaseUrl || this.baseUrl.trim() !== "";
  }

  toUpdate(provider: { id: string; baseUrl?: string }): ProviderUpdate {
    const input: ProviderUpdate = { provider: provider.id };
    if (this.apiKey.trim() !== "") {
      input.apiKey = setProviderSetting(this.apiKey);
    }
    if (this.baseUrl !== (provider.baseUrl ?? "")) {
      const baseUrl = this.baseUrl.trim();
      input.baseUrl = baseUrl === "" ? { type: "clear" } : setProviderSetting(baseUrl);
    }
    return input;
  }
}

function setProviderSetting(value: string): ProviderSettingChange {
  return { type: "set", value };
}
