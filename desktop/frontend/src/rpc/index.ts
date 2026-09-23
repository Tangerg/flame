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
  ClientCapabilities,
  ServerCapabilities,
  FeatureCapability,
  RequestMeta,
  DiscoverResponse,
  Session,
  WorkspaceSummary,
  SessionArtifact,
  SessionSnapshot,
  RunRef,
  RunOutcome,
  SegmentOutcome,
  RunProgress,
  RunMetrics,
  RunProtocolProfile,
  StartRunResponse,
  CancelRunResponse,
  Item,
  ContentBlock,
  Question,
  ToolInvocation,
  RunEvent,
  StreamEvent,
  ItemDelta,
  Interrupt,
  PendingInterruptSet,
  Plan,
  InterruptResponse,
  Goal,
  WorkspaceFileChange,
  PlanStep,
  Usage,
  ProblemData,
  Provider,
  ProviderConfigChange,
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
  errorActiveRun,
  errorRetryAfterSeconds,
} from "./types";
