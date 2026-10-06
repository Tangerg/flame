export interface ProviderRole {
  provider?: string;
  model?: string;
}

export type ProviderCredentialSource = "stored" | "env";
export type ProviderCredentialRequirement = "apiKeyRequired" | "apiKeyOptional";

export interface ProviderConfigurationSnapshot {
  id: string;
  baseUrl?: string;
  credential?: { masked: string; source: ProviderCredentialSource };
  configured: boolean;
  credentialRequirement: ProviderCredentialRequirement;
  requiresBaseUrl?: boolean;
  embeddingCapable?: boolean;
  defaultEmbeddingModel?: string;
}

export class ProviderAuthentication {
  private constructor(readonly requirement: ProviderCredentialRequirement) {}

  static restore(requirement: ProviderCredentialRequirement): ProviderAuthentication {
    if (requirement !== "apiKeyRequired" && requirement !== "apiKeyOptional") {
      throw new Error("provider credential requirement is invalid");
    }
    return new ProviderAuthentication(requirement);
  }

  get requiresAPIKey(): boolean {
    return this.requirement === "apiKeyRequired";
  }
}

export class ProviderCredential {
  private constructor(
    readonly masked: string,
    readonly source: ProviderCredentialSource,
  ) {}

  static configured(masked: string, source: ProviderCredentialSource): ProviderCredential {
    if (masked.trim() === "") throw new Error("provider credential mask is empty");
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
    readonly authentication: ProviderAuthentication,
    readonly requiresBaseUrl: boolean,
    readonly embeddingCapable: boolean,
    readonly defaultEmbeddingModel: string | undefined,
  ) {}

  static restore(snapshot: ProviderConfigurationSnapshot): ProviderConfiguration {
    const authentication = ProviderAuthentication.restore(snapshot.credentialRequirement);
    const credential = snapshot.credential
      ? ProviderCredential.configured(snapshot.credential.masked, snapshot.credential.source)
      : undefined;
    return new ProviderConfiguration(
      snapshot.id,
      snapshot.baseUrl,
      credential,
      snapshot.configured,
      authentication,
      snapshot.requiresBaseUrl ?? false,
      snapshot.embeddingCapable ?? false,
      snapshot.defaultEmbeddingModel,
    );
  }

  get configured(): boolean {
    return this.configuredState;
  }
}
