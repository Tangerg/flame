import type { Provider } from "@flame/runtime-client/wire";
import type {
  ProviderConfigChange,
  UpdateProviderRequest,
} from "@flame/runtime-client/wire";
export class ProviderCredentialsDraft {
  private constructor(
    readonly apiKey: string,
    private readonly endpoint:
      | { readonly type: "unchanged" }
      | { readonly type: "edited"; readonly value: string },
  ) {}

  static initial(): ProviderCredentialsDraft {
    return new ProviderCredentialsDraft("", { type: "unchanged" });
  }

  baseURL(provider: { baseUrl?: string }): string {
    return this.endpoint.type === "edited"
      ? this.endpoint.value
      : (provider.baseUrl ?? "");
  }

  withAPIKey(apiKey: string): ProviderCredentialsDraft {
    return new ProviderCredentialsDraft(apiKey, this.endpoint);
  }

  withBaseURL(baseUrl: string): ProviderCredentialsDraft {
    return new ProviderCredentialsDraft(this.apiKey, {
      type: "edited",
      value: baseUrl,
    });
  }

  settle(submitted: ProviderCredentialsDraft): ProviderCredentialsDraft {
    return new ProviderCredentialsDraft(
      this.apiKey === submitted.apiKey ? "" : this.apiKey,
      this.endpoint === submitted.endpoint
        ? { type: "unchanged" }
        : this.endpoint,
    );
  }

  dirty(provider: { baseUrl?: string }): boolean {
    return (
      this.apiKey.trim() !== "" ||
      (this.endpoint.type === "edited" &&
        this.endpoint.value !== (provider.baseUrl ?? ""))
    );
  }

  endpointPath(
    provider: Pick<Provider, "baseUrl" | "endpoint">,
  ): string | undefined {
    const raw = this.baseURL(provider).trim();
    if (!/^https?:\/\//i.test(raw) || /[\s\\]/u.test(raw)) return undefined;
    try {
      const url = new URL(raw);
      if (
        !/^https?:$/.test(url.protocol) ||
        url.username ||
        url.password ||
        url.href.includes("?") ||
        url.href.includes("#")
      )
        return undefined;
      return (
        url.pathname.replace(/\/+$/, "") +
          (provider.endpoint?.appendsPath ?? "") || "/"
      );
    } catch {
      return undefined;
    }
  }

  endpointError(
    provider: Pick<Provider, "baseUrl" | "requiresBaseUrl" | "endpoint">,
  ): "required" | "invalid" | "suffix" | undefined {
    const raw = this.baseURL(provider).trim();
    if (!raw) return provider.requiresBaseUrl ? "required" : undefined;
    const pathname = this.endpointPath(provider);
    if (pathname === undefined) return "invalid";
    const suffix = provider.endpoint?.requiredSuffix;
    if (!suffix) return undefined;
    try {
      return decodeURIComponent(pathname).endsWith(suffix)
        ? undefined
        : "suffix";
    } catch {
      return "invalid";
    }
  }

  valid(
    provider: Pick<Provider, "baseUrl" | "requiresBaseUrl" | "endpoint">,
  ): boolean {
    return this.endpointError(provider) === undefined;
  }

  toUpdate(provider: { id: string; baseUrl?: string }): UpdateProviderRequest {
    const input: UpdateProviderRequest = { provider: provider.id };
    if (this.apiKey.trim() !== "") {
      input.apiKey = setProviderSetting(this.apiKey);
    }
    if (
      this.endpoint.type === "edited" &&
      this.endpoint.value !== (provider.baseUrl ?? "")
    ) {
      const baseUrl = this.endpoint.value.trim();
      input.baseUrl =
        baseUrl === "" ? { type: "clear" } : setProviderSetting(baseUrl);
    }
    return input;
  }
}

function setProviderSetting(value: string): ProviderConfigChange {
  return { type: "set", value };
}
