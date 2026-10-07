export interface ProviderRole {
  provider?: string;
  model?: string;
}

export type ProviderCredentialSource = "stored" | "env";

export interface ProviderConfigurationSnapshot {
  id: string;
  baseUrl?: string;
  credential?: { masked: string; source: ProviderCredentialSource };
  configured: boolean;
  requiresBaseUrl?: boolean;
  embeddingCapable?: boolean;
  defaultEmbeddingModel?: string;
}

export class ProviderCredential {
  private constructor(
    readonly masked: string,
    readonly source: ProviderCredentialSource,
  ) {}

  static configured(masked: string, source: ProviderCredentialSource): ProviderCredential {
    return new ProviderCredential(masked, source);
  }

  get fromEnvironment(): boolean {
    return this.source === "env";
  }

  get stored(): boolean {
    return this.source === "stored";
  }
}

export class ProviderConfiguration {
  private constructor(
    readonly id: string,
    readonly baseUrl: string | undefined,
    readonly credential: ProviderCredential | undefined,
    private readonly configuredState: boolean,
    readonly requiresBaseUrl: boolean,
    readonly embeddingCapable: boolean,
    readonly defaultEmbeddingModel: string | undefined,
  ) {}

  static restore(snapshot: ProviderConfigurationSnapshot): ProviderConfiguration {
    const credential = snapshot.credential
      ? ProviderCredential.configured(snapshot.credential.masked, snapshot.credential.source)
      : undefined;
    return new ProviderConfiguration(
      snapshot.id,
      snapshot.baseUrl,
      credential,
      snapshot.configured,
      snapshot.requiresBaseUrl ?? false,
      snapshot.embeddingCapable ?? false,
      snapshot.defaultEmbeddingModel,
    );
  }

  get configured(): boolean {
    return this.configuredState;
  }
}
