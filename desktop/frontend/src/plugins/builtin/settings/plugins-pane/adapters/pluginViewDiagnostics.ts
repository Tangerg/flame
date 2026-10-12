import { RpcError, RpcProtocolError, RpcTransportError } from "@flame/runtime-contract/client";
import { reportPluginError } from "@/plugins/sdk/errors";
import type { PluginViewReads, PluginViewStage } from "../application/pluginView";

export function reportViewFailure(
  binding: PluginViewReads<unknown>["binding"],
  stage: PluginViewStage,
  cause: unknown,
): void {
  const problem =
    cause instanceof RpcError
      ? { category: cause.data.type, requestId: cause.requestId }
      : cause instanceof RpcProtocolError
        ? { category: "invalid_response", requestId: cause.requestId }
        : cause instanceof RpcTransportError
          ? { category: "transport", status: cause.status, requestId: cause.requestId }
          : {
              category:
                cause instanceof TypeError
                  ? "type_error"
                  : cause instanceof SyntaxError
                    ? "invalid_json"
                    : "internal_error",
            };
  // Raw integration messages can contain credentials or private document content.
  reportPluginError(
    binding.installationId,
    "render",
    new Error(`plugin view ${stage}: ${problem.category}`),
    JSON.stringify({ binding, stage, problem }),
  );
}
