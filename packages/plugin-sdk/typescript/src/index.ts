export {
  Capability,
  Method,
  PROTOCOL,
  canonicalJSON,
  digestJSON,
  validCommand,
  type CapabilityDescriptor,
  type MethodDescriptor,
} from "./contract";
export {
  ExactNumber,
  parseJSON,
  stringifyJSON,
  type JSONNumber,
} from "@flame/runtime-client";
export {
  Backend,
  type CallContext,
  type SetupContext,
  type SetupContribution,
  type SetupBinding,
  type ActivationResource,
  type StreamFrame,
  type ReceiptRequest,
  type ToolReceipt,
} from "./backend";
export {
  Refusal,
  decodeReply,
  connectCallback,
  callHost,
  type CallbackRef,
  type Invocation,
} from "./callback";
