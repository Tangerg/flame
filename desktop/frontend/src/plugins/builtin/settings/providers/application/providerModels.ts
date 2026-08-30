import { validateModelIdentity, validateProviderIdentity } from "./modelIdentity";

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

/** A credential summary describes actual secret material, never provider readiness. */
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

/** Immutable provider configuration used by every Desktop consumer. */
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
    validateProviderIdentity(snapshot.id);
    if (snapshot.baseUrl !== undefined && snapshot.baseUrl.trim() === "") {
      throw new Error("provider base URL is empty");
    }
    const authentication = ProviderAuthentication.restore(snapshot.credentialRequirement);
    const credential = snapshot.credential
      ? ProviderCredential.configured(snapshot.credential.masked, snapshot.credential.source)
      : undefined;
    if (snapshot.configured && authentication.requiresAPIKey && credential === undefined) {
      throw new Error("configured provider is missing its required API key");
    }
    if (snapshot.configured && snapshot.requiresBaseUrl && snapshot.baseUrl === undefined) {
      throw new Error("configured provider is missing its required base URL");
    }
    if (snapshot.defaultEmbeddingModel !== undefined && !snapshot.embeddingCapable) {
      throw new Error("provider without embeddings carries a default embedding model");
    }
    if (snapshot.defaultEmbeddingModel !== undefined) {
      validateModelIdentity(snapshot.defaultEmbeddingModel);
    }
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
