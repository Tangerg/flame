import { useMemo, type ComponentProps } from "react";
import type { CapturedMessage } from "@flame/component-sdk";
import { pluginContract } from "@flame/runtime-client";
import {
  useCurrentMessage,
  useCurrentMessageSessionId,
} from "@/features/chat/public/messageContext";
import { usePluginProviders } from "@/features/plugins/adapters/packages";
import { RenderingComponent } from "./InstalledComponent";

export function InstalledMessageAction(props: ComponentProps<typeof RenderingComponent>) {
  const message = useCurrentMessage();
  const sessionId = useCurrentMessageSessionId();
  const choices = usePluginProviders();
  const input = useMemo<CapturedMessage>(
    () =>
      Object.freeze({
        sessionId,
        itemId: message.id,
        runId: message.runId || undefined,
        role: message.role,
      }),
    [sessionId, message.id, message.runId, message.role],
  );
  const binding = props.binding;
  if (binding) {
    const descriptor = pluginContract(binding.contract);
    if (
      descriptor.composition === "selection" &&
      !choices.data?.some(
        (choice) =>
          choice.role === descriptor.reference &&
          choice.installationId === binding.installationId &&
          choice.contribution === binding.contribution,
      )
    )
      return null;
  }
  return (
    <RenderingComponent
      key={JSON.stringify([sessionId, message.id, message.runId])}
      {...props}
      message={input}
      sessionId={sessionId}
    />
  );
}
