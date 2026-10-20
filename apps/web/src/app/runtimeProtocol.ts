import { CLIENT_VERSION, PRODUCT_SLUG } from "@/app/product";
import { PROTOCOL_VERSION } from "@flame/runtime-client";
import { type ClientCapabilities, type RequestMeta } from "@flame/runtime-client";
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
