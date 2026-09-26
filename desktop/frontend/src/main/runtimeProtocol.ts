import { CLIENT_VERSION } from "@/main/config";
import { PRODUCT_SLUG } from "@/product";
import { PROTOCOL_VERSION } from "@flame/runtime-contract/client";
import type { ClientCapabilities, RequestMeta } from "@flame/runtime-contract/client";

export const CLIENT_CAPABILITIES: ClientCapabilities = {
  features: {
    multimodal: { enabled: true },
    subagents: { enabled: true },
  },
  interruptTypes: ["approval", "question"],
};

export function runtimeRequestMeta(surface: "desktop" | "web"): RequestMeta {
  return {
    protocolVersion: PROTOCOL_VERSION,
    clientInfo: { name: `${PRODUCT_SLUG}-${surface}`, version: CLIENT_VERSION },
    clientCapabilities: CLIENT_CAPABILITIES,
  };
}
