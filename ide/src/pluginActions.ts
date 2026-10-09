import * as vscode from "vscode";
import type {
  PluginAction,
  RenamePluginSessionRequest,
  Session,
} from "@flame/runtime-contract/wire";
import { commandFailureMessage, type CommandResult, type Connection } from "./connection";

export async function runPluginAction(
  connection: Connection,
  session: Session,
): Promise<CommandResult | undefined> {
  const target = {
    sessionId: session.id,
    expectedRevision: session.revision,
    title: session.title,
  };
  const cancellation = new vscode.CancellationTokenSource();
  const retire = () => cancellation.cancel();
  connection.signal.addEventListener("abort", retire, { once: true });
  try {
    if (connection.signal.aborted) return;
    const installed = await connection.client.plugins.list(connection.signal);
    if (connection.signal.aborted) return;
    const choices = installed.data
      .filter(
        (installation) =>
          installation.state === "enabled" && installation.presentation === "admitted",
      )
      .flatMap((installation) =>
        installation.selected.actions.map((action) => ({
          label: action.title,
          description: installation.selected.name,
          detail: `Rename Session “${target.title || target.sessionId}”`,
          action,
          request: {
            installationId: installation.id,
            digest: installation.selected.digest,
            actionId: action.id,
            update: target,
          } satisfies RenamePluginSessionRequest,
        })),
      );
    if (choices.length === 0)
      throw new Error("no enabled plugin declares an available Session action");
    const picked = await vscode.window.showQuickPick(
      choices,
      {
        title: "Run Plugin Action",
        placeHolder: "Choose an action for the selected Session",
      },
      cancellation.token,
    );
    if (!picked || connection.signal.aborted) return;
    return await renameForm(connection, picked.action, picked.request, cancellation.token);
  } catch (error) {
    if (connection.signal.aborted) return;
    throw error;
  } finally {
    connection.signal.removeEventListener("abort", retire);
    cancellation.dispose();
  }
}

function renameForm(
  connection: Connection,
  action: PluginAction,
  request: RenamePluginSessionRequest,
  cancellation: vscode.CancellationToken,
): Promise<CommandResult | undefined> {
  if (cancellation.isCancellationRequested) return Promise.resolve(undefined);
  const input = vscode.window.createInputBox();
  input.title = `${action.title} · Rename Session`;
  input.prompt = `New title for “${request.update.title || request.update.sessionId}” · ${request.update.sessionId}`;
  input.value = request.update.title;
  input.ignoreFocusOut = true;
  return new Promise((resolve) => {
    let closed = false;
    const subscriptions: vscode.Disposable[] = [];
    const finish = (result?: CommandResult) => {
      if (closed) return;
      closed = true;
      for (const subscription of subscriptions) subscription.dispose();
      input.hide();
      input.dispose();
      resolve(result);
    };
    const submit = async () => {
      if (closed || input.busy || cancellation.isCancellationRequested) return;
      const title = input.value;
      input.busy = true;
      input.enabled = false;
      let result: CommandResult;
      try {
        result = await connection.execute({
          method: "plugins.renameSession",
          params: { ...request, update: { ...request.update, title } },
        });
      } catch (error) {
        if (!closed && !cancellation.isCancellationRequested) {
          input.validationMessage = commandFailureMessage(error);
          input.busy = false;
          input.enabled = true;
        }
        return;
      }
      if (!closed && !cancellation.isCancellationRequested) finish(result);
    };
    subscriptions.push(
      input.onDidAccept(() => void submit()),
      input.onDidHide(() => finish()),
      input.onDidChangeValue(() => {
        input.validationMessage = undefined;
      }),
      cancellation.onCancellationRequested(() => finish()),
    );
    input.show();
  });
}
