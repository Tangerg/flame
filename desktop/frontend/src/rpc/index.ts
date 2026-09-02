// Public surface of the Flame Runtime Protocol client (runtime/doc/API.md). Sidecar
// metadata is HTTP-only and lives behind `createSidecarClient`.

export {
  isErrorType,
  RpcConnectionError,
  RpcError,
  RpcProtocolError,
  RpcTransportError,
} from "./errors";
export { asItemId, asRunId, asSegmentId, asSessionId } from "./ids";
export type { ItemId, RunId, SegmentId } from "./ids";
export type { MutationAttemptOptions, MutationPromise } from "./mutation";
export { createMutationJournal } from "./mutationJournal";
export {
  createMutationSettler,
  MUTATION_ATTEMPT_TIMEOUT_MS,
  MutationSettlementClosedError,
} from "./mutationSettlement";
export type { MutationSettler } from "./mutationSettlement";
export type { Methods, StreamingResult } from "./methods";
export { createFlameClient } from "./sdk";
export type { FlameClient } from "./sdk";
export { HTTP_ENDPOINTS, PROTOCOL_VERSION } from "@flame/runtime-contract/wire";
export type {
  // Lifecycle / capabilities
  ClientCapabilities,
  ServerCapabilities,
  FeatureCapability,
  RequestMeta,
  DiscoverResponse,
  // Sessions / workspaces
  Session,
  WorkspaceSummary,
  SessionArtifact,
  SessionSnapshot,
  // Runs
  RunRef,
  RunOutcome,
  SegmentOutcome,
  RunProgress,
  RunMetrics,
  RunProtocolProfile,
  StartRunResponse,
  CancelRunResponse,
  // Items
  Item,
  ContentBlock,
  Question,
  ToolInvocation,
  // Streaming
  RunEvent,
  StreamEvent,
  ItemDelta,
  // HITL
  Interrupt,
  PendingInterruptSet,
  Plan,
  InterruptResponse,
  Goal,
  // Files
  WorkspaceFileChange,
  // Plan
  PlanStep,
  // Usage / error
  Usage,
  ProblemData,
  // Providers
  Provider,
  ProviderConfigChange,
  // Workspace optional domains
  Schedule,
  CreateScheduleRequest,
  AgentMemoryItem,
  MCPServer,
  MCPHandshakeTimeout,
  MCPAuthorizationAttempt,
  MCPConnectionInput,
  MCPAuthorizationChange,
  MCPEnvironmentChange,
  MCPHeadersChange,
  MCPServerCandidate,
  UpdateMCPServerRequest,
  KnowledgeEntry,
  RuntimeEvent,
  RuntimeTopic,
} from "@flame/runtime-contract/wire";
export type { WireFeature } from "@flame/runtime-contract/methods";
export { createSidecarClient } from "./sidecar";
export type { LivenessStatus, ReadinessStatus, RuntimeInfo, SidecarClient } from "./sidecar";
export { createDesktopHostClient } from "./desktopHost";
export type { DesktopBootstrap, DesktopHostClient } from "./desktopHost";
export { createHttpTransport } from "./transports/http";
export {
  JSONRPC_VERSION,
  RPC_METHOD_NOT_FOUND,
  errorType,
  errorDetail,
  errorRetryAfterSeconds,
} from "./types";
