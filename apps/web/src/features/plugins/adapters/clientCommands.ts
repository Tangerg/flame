import {
  type ClientCommand,
  type ComponentContext,
  type ComponentReference,
} from "@flame/component-sdk";
import { type ContributionLifetime } from "@/extensions/definePlugin";
import { trackContributionIO } from "@/extensions/contributionIO";
import { reportPluginError } from "@/extensions/errors";
import { COMMAND } from "@/features/command/public/extensionPoints";
import { notifyFrom } from "@/features/notifications/public/notifications";
import { failureMessage } from "@/lib/diagnostics";

export function createClientCommandRegistration(
  lifetime: ContributionLifetime,
  signal: AbortSignal,
  component: ComponentReference,
  captureSessionId: () => string | undefined,
): ComponentContext["registerCommand"] {
  const registered = new Map<string, symbol>();
  const source = `package:${component.installationId}`;
  return (command: ClientCommand) => {
    signal.throwIfAborted();
    const { id: localId, label, combo, run } = command;
    if (!localId.trim() || !label.trim() || (combo !== undefined && !combo.trim()))
      throw new Error("Client command IDs, labels and declared shortcuts must not be empty.");
    if (registered.has(localId))
      throw new Error(`Client command "${localId}" is already registered.`);
    const registration = Symbol(localId);
    registered.set(localId, registration);
    const execute = async () => {
      signal.throwIfAborted();
      if (registered.get(localId) !== registration)
        throw new Error("The client command has been withdrawn.");
      const sessionId = captureSessionId() || undefined;
      await trackContributionIO(lifetime, async () => run({ sessionId, signal }));
    };
    const contribution = lifetime.contribute(COMMAND, {
      id: JSON.stringify(["package", component.installationId, component.componentId, localId]),
      label,
      combo,
      run: () => {
        const work = execute();
        void work.catch((error: unknown) => {
          if (signal.aborted) return;
          reportPluginError(source, "command", error);
          notifyFrom(source, `${label}: ${failureMessage(error)}`, "error");
        });
        return work;
      },
    });
    const stop = () => {
      if (registered.get(localId) !== registration) return;
      registered.delete(localId);
      signal.removeEventListener("abort", stop);
      contribution.dispose();
    };
    signal.addEventListener("abort", stop, { once: true });
    return stop;
  };
}
