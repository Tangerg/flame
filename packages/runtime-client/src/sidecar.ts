import { requireRuntimeEndpoint } from "./endpoint";
import {
  errorMessage,
  parseTransportProblem,
  RpcConnectionError,
  RpcProtocolError,
  RpcTransportError,
} from "./errors";
import { parseJSON } from "./json";
import {
  projectHTTPSidecarResponse,
  validateHTTPSidecarResponse,
} from "./generated/wire.validate.generated";
import {
  HTTP_ENDPOINTS,
  type HTTPSidecarEndpointName,
  type HTTPSidecarResponses,
  type LivenessStatus,
  type ReadinessStatus,
  type RuntimeInfo,
} from "@flame/runtime-client/wire";

export type { LivenessStatus, ReadinessStatus, RuntimeInfo } from "@flame/runtime-client/wire";

export interface SidecarClientConfig {
  baseUrl: string;
  fetch?: typeof fetch;
}

export interface SidecarClient {
  info(signal?: AbortSignal): Promise<RuntimeInfo>;
  liveness(signal?: AbortSignal): Promise<LivenessStatus>;
  readiness(signal?: AbortSignal): Promise<ReadinessStatus>;
}

export function createSidecarClient(config: SidecarClientConfig): SidecarClient {
  const baseUrl = requireRuntimeEndpoint(config.baseUrl);
  const fetchImpl = config.fetch ?? globalThis.fetch.bind(globalThis);

  async function getJson<Endpoint extends HTTPSidecarEndpointName>(
    endpoint: Endpoint,
    signal?: AbortSignal,
  ): Promise<HTTPSidecarResponses[Endpoint]> {
    const specification = HTTP_ENDPOINTS[endpoint];
    const path = specification.path;
    let res: Response;
    try {
      res = await fetchImpl(`${baseUrl}${path}`, {
        method: specification.method,
        redirect: "error",
        headers: { Accept: "application/json" },
        signal,
      });
    } catch (err) {
      throw new RpcConnectionError(`sidecar ${path}: ${errorMessage(err)}`);
    }
    const requestId = res.headers.get("Request-Id") ?? undefined;
    let bytes: ArrayBuffer;
    try {
      bytes = await res.arrayBuffer();
    } catch (err) {
      throw new RpcConnectionError(
        `sidecar ${path}: response could not be read: ${errorMessage(err)}`,
        requestId,
      );
    }
    let text: string;
    try {
      text = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    } catch (cause) {
      const error = new RpcProtocolError(
        `sidecar ${path} response`,
        [{ path: "$", detail: "must contain valid UTF-8" }],
        requestId,
      );
      error.cause = cause;
      throw error;
    }
    if (!specification.responseStatuses.some((status) => status === res.status)) {
      const problem = parseTransportProblem(text);
      const requestId = problem?.requestId ?? res.headers.get("Request-Id") ?? undefined;
      const detail = problem?.detail || res.statusText || "sidecar request failed";
      throw new RpcTransportError(
        `sidecar ${path}: http ${res.status}: ${detail}`,
        res.status,
        requestId,
        problem?.type,
      );
    }
    let json: unknown;
    try {
      json = parseJSON(text);
    } catch (err) {
      throw new RpcProtocolError(
        `sidecar ${path} response`,
        [{ path: "$", detail: `must contain valid JSON: ${errorMessage(err)}` }],
        requestId,
      );
    }
    const violations = validateHTTPSidecarResponse(endpoint, json);
    if (violations.length > 0) {
      throw new RpcProtocolError(`sidecar ${path} response`, violations, requestId);
    }
    try {
      return projectHTTPSidecarResponse(endpoint, json) as HTTPSidecarResponses[Endpoint];
    } catch (cause) {
      const error = new RpcProtocolError(
        `sidecar ${path} response`,
        [{ path: "$", detail: "number cannot be represented by the typed wire projection" }],
        requestId,
      );
      error.cause = cause;
      throw error;
    }
  }

  return {
    info: (signal) => getJson("info", signal),
    liveness: (signal) => getJson("liveness", signal),
    readiness: (signal) => getJson("readiness", signal),
  };
}
