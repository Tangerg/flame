import * as vscode from "vscode";
import { randomUUID } from "node:crypto";
import { join } from "node:path";
import { asSessionId, normalizeRuntimeEndpoint } from "@flame/runtime-contract/client";
import type {
  Interrupt,
  InterruptResponse,
  Item,
  QuestionField,
  Session,
  SessionSnapshot,
} from "@flame/runtime-contract/wire";
import { Connection } from "./connection";
import type { Command } from "./commandStore";
import { inputFromEditor, type EditorSnapshot } from "./editorContext";
import { observeRun } from "./observation";

class ReadonlyDocuments implements vscode.TextDocumentContentProvider {
  readonly #contents = new Map<string, string>();

  create(name: string, content: string): vscode.Uri {
    const uri = vscode.Uri.from({
      scheme: "flame-content",
      path: `/${randomUUID()}/${name.replaceAll("/", "_")}`,
    });
    this.#contents.set(uri.toString(), content);
    return uri;
  }

  provideTextDocumentContent(uri: vscode.Uri): string {
    const content = this.#contents.get(uri.toString());
    if (content === undefined) throw new Error("Flame snapshot is no longer available");
    return content;
  }

  close(uri: vscode.Uri): void {
    this.#contents.delete(uri.toString());
  }
}

class Workbench implements vscode.TreeDataProvider<Session> {
  readonly #context: vscode.ExtensionContext;
  readonly #changes = new vscode.EventEmitter<void>();
  readonly #output = vscode.window.createOutputChannel("Flame");
  readonly #status = vscode.window.createStatusBarItem(vscode.StatusBarAlignment.Left);
  readonly #documents = new ReadonlyDocuments();
  readonly #disposables: vscode.Disposable[] = [];
  readonly #tasks = new Set<Promise<void>>();
  readonly onDidChangeTreeData = this.#changes.event;
  #connection?: Connection;
  #sessions: Session[] = [];
  #session?: Session;
  #observation?: AbortController;
  #connecting?: AbortController;
  #refreshing: Promise<void> = Promise.resolve();
  #submitted?: EditorSnapshot;
  #closed = false;

  constructor(context: vscode.ExtensionContext) {
    this.#context = context;
    this.#status.text = "$(plug) Flame: Connect";
    this.#status.command = "flame.connect";
    this.#status.show();
    this.#disposables.push(
      this.#changes,
      this.#output,
      this.#status,
      vscode.window.registerTreeDataProvider("flame.sessions", this),
      vscode.workspace.registerTextDocumentContentProvider("flame-content", this.#documents),
      vscode.workspace.onDidCloseTextDocument((document) => this.#documents.close(document.uri)),
    );
    this.#register("connect", () => this.#connect());
    this.#register("disconnect", () => this.#disconnect());
    this.#register("selectSession", (session?: Session) => this.#selectSession(session));
    this.#register("createSession", () => this.#createSession());
    this.#register("sendPrompt", () => this.#send(false));
    this.#register("sendSelection", () => this.#send(true));
    this.#register("resume", () => this.#resume());
    this.#register("cancelRun", () => this.#cancel());
    this.#register("retryCommand", () => this.#retry());
    this.#register("refresh", () => this.#refresh());
    this.#register("reviewChanges", () => this.#reviewChanges());
    this.#register("openRuntimeFile", () => this.#openRuntimeFile());
    this.#register("compareContext", () => this.#compareContext());
  }

  getTreeItem(session: Session): vscode.TreeItem {
    const item = new vscode.TreeItem(session.title || session.id);
    item.id = session.id;
    item.description = session.status;
    item.tooltip = `${session.workspace.ref.path}\n${session.id}`;
    item.command = {
      command: "flame.selectSession",
      title: "Select Session",
      arguments: [session],
    };
    item.iconPath = new vscode.ThemeIcon(
      session.status === "waiting"
        ? "question"
        : session.status === "running"
          ? "sync~spin"
          : "comment",
    );
    return item;
  }

  getChildren(): Session[] {
    return this.#sessions;
  }

  #register(name: string, run: (session?: Session) => Promise<void>): void {
    this.#disposables.push(
      vscode.commands.registerCommand(`flame.${name}`, (session?: Session) =>
        this.#track(async () => {
          if (this.#closed) return;
          try {
            await run(session);
          } catch (error) {
            if (!this.#closed)
              await vscode.window.showErrorMessage(
                `Flame: ${error instanceof Error ? error.message : String(error)}`,
              );
          }
        }),
      ),
    );
  }

  #track(run: () => Promise<void>): Promise<void> {
    const task = run();
    this.#tasks.add(task);
    void task.then(
      () => this.#tasks.delete(task),
      () => this.#tasks.delete(task),
    );
    return task;
  }

  #connected(): Connection {
    if (!this.#connection) throw new Error("connect to a Runtime first");
    return this.#connection;
  }

  #selected(): Session {
    if (!this.#session) throw new Error("select or create a Session first");
    return this.#session;
  }

  async #connect(): Promise<void> {
    const input = await vscode.window.showInputBox({
      title: "Connect to Flame Runtime",
      prompt: "Address of the running Runtime",
      value: vscode.workspace.getConfiguration("flame").get("endpoint"),
      ignoreFocusOut: true,
    });
    if (input === undefined) return;
    const endpoint = normalizeRuntimeEndpoint(input);
    if (!endpoint)
      throw new Error("enter an HTTP(S) address without credentials, query, or fragment");
    const key = `flame.runtimeToken:${endpoint}`;
    const existing = await this.#context.secrets.get(key);
    const token = await vscode.window.showInputBox({
      title: "Runtime local token",
      prompt:
        "Token for this exact Runtime address; leave empty if the gateway handles authentication",
      value: existing ?? "",
      password: true,
      ignoreFocusOut: true,
    });
    if (token === undefined) return;
    await this.#disconnect();
    const connecting = new AbortController();
    this.#connecting = connecting;
    let connection: Connection;
    try {
      connection = await Connection.open(
        endpoint,
        token.trim() || undefined,
        join(this.#context.globalStorageUri.fsPath, "commands"),
        AbortSignal.any([connecting.signal, AbortSignal.timeout(30_000)]),
      );
    } catch (error) {
      if (this.#connecting === connecting) this.#connecting = undefined;
      throw error;
    }
    if (this.#closed || connecting.signal.aborted || this.#connecting !== connecting) {
      await connection.close();
      return;
    }
    this.#connecting = undefined;
    this.#connection = connection;
    if (token.trim()) await this.#context.secrets.store(key, token.trim());
    else await this.#context.secrets.delete(key);
    if (this.#connection !== connection) return;
    await vscode.workspace
      .getConfiguration("flame")
      .update("endpoint", endpoint, vscode.ConfigurationTarget.Global);
    if (this.#connection !== connection) return;
    this.#status.command = "flame.selectSession";
    this.#status.text = "$(flame) Flame: Select Session";
    await this.#refresh();
    if (this.#connection !== connection) return;
    this.#track(async () => {
      try {
        const subscription = await connection.client.runtimeEvents.subscribe(
          { topics: ["sessions.changed", "runs.changed", "interrupts.changed"] },
          connection.signal,
        );
        for await (const _event of subscription.events) {
          if (this.#connection !== connection || connection.signal.aborted) return;
          await this.#refresh();
        }
      } catch (error) {
        if (this.#connection === connection && !connection.signal.aborted) {
          this.#status.text = "$(warning) Flame: Refresh connection";
          this.#output.appendLine(
            `Runtime observation ended: ${error instanceof Error ? error.message : String(error)}. Connect again to restore notifications.`,
          );
        }
      }
    });
    if (connection.pendingCommands().length > 0)
      await vscode.window.showWarningMessage(
        "Flame has an unresolved command. Use Retry Unresolved Command to recover the exact saved input.",
      );
  }

  async #disconnect(): Promise<void> {
    const connection = this.#connection;
    this.#connecting?.abort();
    this.#connecting = undefined;
    this.#connection = undefined;
    this.#observation?.abort();
    this.#observation = undefined;
    this.#sessions = [];
    this.#session = undefined;
    this.#submitted = undefined;
    this.#changes.fire();
    this.#status.text = "$(plug) Flame: Connect";
    this.#status.command = "flame.connect";
    if (connection) await connection.close();
  }

  async #selectSession(session?: Session): Promise<void> {
    const connection = this.#connected();
    const selected =
      session ??
      (
        await vscode.window.showQuickPick(
          this.#sessions.map((value) => ({
            label: value.title || value.id,
            description: `${value.status} · ${value.workspace.ref.path}`,
            value,
          })),
          { title: "Flame Session" },
        )
      )?.value;
    if (!selected || this.#connection !== connection) return;
    this.#session = selected;
    await this.#refresh();
    this.#output.show(true);
  }

  async #createSession(): Promise<void> {
    const connection = this.#connected();
    const workspaces = await connection.client.workspaces.list(connection.signal);
    const choices = [
      ...workspaces.data.map((workspace) => ({
        label: workspace.name,
        description: workspace.workspace.ref.path,
        path: workspace.workspace.ref.path,
      })),
      {
        label: "Enter Runtime workspace path…",
        description: "Path on the Runtime host",
        path: undefined,
      },
    ];
    const chosen = await vscode.window.showQuickPick(choices, {
      title: "Create Flame Session",
      placeHolder: "Choose a workspace on the Runtime host",
    });
    if (!chosen) return;
    const path =
      chosen.path ??
      (await vscode.window.showInputBox({
        title: "Runtime workspace path",
        prompt: "This path is resolved by Runtime, independently of the editor filesystem",
        value: connection.discovery.serverInfo.defaultWorkspace.path,
        ignoreFocusOut: true,
      }));
    if (!path?.trim() || this.#connection !== connection) return;
    await this.#execute({ kind: "createSession", params: { workspace: { path: path.trim() } } });
  }

  async #send(withEditor: boolean): Promise<void> {
    const connection = this.#connected();
    const session = this.#selected();
    let snapshot: EditorSnapshot | undefined;
    if (withEditor) {
      const editor = vscode.window.activeTextEditor;
      if (!editor) throw new Error("open a source document first");
      const document = editor.document;
      snapshot = {
        uri: document.uri.toString(),
        documentVersion: document.version,
        languageId: document.languageId,
        dirty: document.isDirty,
        selection: {
          start: { line: editor.selection.start.line, character: editor.selection.start.character },
          end: { line: editor.selection.end.line, character: editor.selection.end.character },
        },
        text: document.getText(),
      };
    }
    const prompt = await vscode.window.showInputBox({
      title: "Send to Flame",
      prompt: snapshot
        ? "Instructions for the captured editor selection"
        : "Instructions for the selected Runtime Session",
      ignoreFocusOut: true,
    });
    if (!prompt?.trim() || this.#connection !== connection || this.#session?.id !== session.id)
      return;
    if (snapshot) this.#submitted = snapshot;
    await this.#execute({
      kind: "start",
      params: { sessionId: session.id, input: inputFromEditor(prompt, snapshot) },
    });
  }

  async #execute(command: Command): Promise<void> {
    const connection = this.#connected();
    const result = await connection.execute(command);
    if (this.#connection !== connection) return;
    if (result.sessionId) {
      const session = await connection.client.sessions.get(
        asSessionId(result.sessionId),
        connection.signal,
      );
      if (this.#connection !== connection) return;
      this.#session = session;
    }
    await this.#refresh();
    this.#output.show(true);
  }

  async #retry(): Promise<void> {
    const connection = this.#connected();
    const selected = await vscode.window.showQuickPick(
      connection.pendingCommands().map((pending) => ({
        label: pending.command.kind,
        description: pending.id,
        detail: JSON.stringify(pending.command.params),
        pending,
      })),
      {
        title: "Retry exact saved command",
        placeHolder: "Each command retains its original input and Runtime store",
      },
    );
    if (!selected || this.#connection !== connection) return;
    const result = await connection.retry(selected.pending.id);
    if (this.#connection !== connection) return;
    if (result.sessionId) {
      const session = await connection.client.sessions.get(
        asSessionId(result.sessionId),
        connection.signal,
      );
      if (this.#connection !== connection) return;
      this.#session = session;
    }
    await this.#refresh();
  }

  async #resume(): Promise<void> {
    const connection = this.#connected();
    const session = this.#selected();
    const waiting = await connection.client.interrupts.list(
      { sessionId: asSessionId(session.id) },
      connection.signal,
    );
    const selected = await vscode.window.showQuickPick(
      waiting.data.map((value) => ({
        label: value.rootRunId,
        description: `${value.interrupts.length} pending responses`,
        value,
      })),
      { title: "Respond to waiting Run" },
    );
    if (!selected) return;
    const responses: InterruptResponse[] = [];
    for (const interrupt of selected.value.interrupts) {
      const response = await this.#answer(interrupt);
      if (!response || this.#connection !== connection) return;
      responses.push(response);
    }
    await this.#execute({ kind: "resume", params: { runId: selected.value.rootRunId, responses } });
  }

  async #answer(interrupt: Interrupt): Promise<InterruptResponse | undefined> {
    if (interrupt.type === "approval") {
      this.#output.appendLine(
        `Approval request: ${interrupt.payload.tool.name}\n${JSON.stringify(interrupt.payload.tool.arguments, null, 2)}`,
      );
      this.#output.show(true);
      const decision = await vscode.window.showWarningMessage(
        `${interrupt.payload.tool.name}${interrupt.payload.risk ? ` (${interrupt.payload.risk})` : ""}`,
        {
          modal: true,
          detail: `${interrupt.payload.reason ?? "Review the tool arguments in Flame output."}\n\n${JSON.stringify(interrupt.payload.tool.arguments, null, 2)}`,
        },
        "Approve",
        "Deny",
      );
      if (!decision) return;
      return {
        itemId: interrupt.itemId,
        response: { type: "approval", decision: decision === "Approve" ? "approve" : "deny" },
      };
    }
    const answers: string[][] = [];
    for (const field of interrupt.payload.question.fields) {
      const answer = await this.#question(field);
      if (answer === undefined) return;
      answers.push(answer);
    }
    return { itemId: interrupt.itemId, response: { type: "answer", answers } };
  }

  async #question(field: QuestionField): Promise<string[] | undefined> {
    if (field.type === "text") {
      const answer = await vscode.window.showInputBox({
        title: field.header ?? "Flame question",
        prompt: field.prompt,
        ignoreFocusOut: true,
      });
      return answer === undefined ? undefined : [answer];
    }
    const options = field.options.map((option) => ({
      label: option.label,
      description: option.description,
      detail: option.preview,
      custom: false,
    }));
    if (field.allowCustom)
      options.push({
        label: "Enter another answer…",
        description: undefined,
        detail: undefined,
        custom: true,
      });
    const chosen = await vscode.window.showQuickPick(options, {
      title: field.header,
      placeHolder: field.prompt,
      canPickMany: field.multiple,
      ignoreFocusOut: true,
    });
    if (!chosen) return;
    const values = Array.isArray(chosen) ? chosen : [chosen];
    const answers = values.filter((value) => !value.custom).map((value) => value.label);
    if (values.some((value) => value.custom)) {
      const custom = await vscode.window.showInputBox({
        prompt: field.prompt,
        ignoreFocusOut: true,
      });
      if (custom === undefined) return;
      answers.push(custom);
    }
    return answers;
  }

  async #cancel(): Promise<void> {
    const connection = this.#connected();
    const session = this.#selected();
    const snapshot = await connection.client.sessions.snapshot(
      asSessionId(session.id),
      true,
      connection.signal,
    );
    const selected = await vscode.window.showQuickPick(
      snapshot.runs
        .filter((run) => run.status !== "finished")
        .map((run) => ({ label: run.id, description: run.status, run })),
      { title: "Cancel Run" },
    );
    if (!selected || this.#connection !== connection) return;
    await this.#execute({
      kind: "cancel",
      params: { runId: selected.run.id, reason: "Canceled from the IDE" },
    });
  }

  #refresh(): Promise<void> {
    const connection = this.#connected();
    const refresh = async () => {
      if (this.#connection !== connection || connection.signal.aborted) return;
      const sessions = await connection.client.sessions
        .list({}, connection.signal)
        .autoPagingToArray();
      if (this.#connection !== connection) return;
      this.#sessions = sessions;
      this.#changes.fire();
      const session = this.#session;
      if (!session) return;
      this.#observation?.abort();
      const observation = new AbortController();
      this.#observation = observation;
      const signal = AbortSignal.any([connection.signal, observation.signal]);
      const [current, snapshot] = await Promise.all([
        connection.client.sessions.get(asSessionId(session.id), signal),
        connection.client.sessions.snapshot(asSessionId(session.id), true, signal),
      ]);
      if (signal.aborted || this.#session?.id !== session.id) return;
      this.#session = current;
      this.#render(snapshot);
      this.#status.text = `$(flame) ${current.title || "Flame"}: ${current.status}`;
      const run = snapshot.runs.find((value) => value.status === "running" && !value.parentRunId);
      if (!run) return;
      this.#track(async () => {
        try {
          await observeRun(
            connection.client,
            run.id,
            {
              snapshot: (value) => {
                if (!signal.aborted) this.#render(value);
              },
              event: (value) => {
                if (!signal.aborted && value.event.type === "item.completed")
                  this.#output.appendLine(this.#item(value.event.item));
              },
            },
            signal,
          );
        } catch (error) {
          if (!signal.aborted)
            this.#output.appendLine(
              `Run observation ended: ${error instanceof Error ? error.message : String(error)}. Refresh Session to reattach from an authoritative snapshot.`,
            );
        }
      });
    };
    this.#refreshing = this.#refreshing.catch(() => undefined).then(refresh);
    return this.#refreshing;
  }

  #render(snapshot: SessionSnapshot): void {
    this.#output.clear();
    this.#output.appendLine(`${this.#selected().title} · ${this.#selected().workspace.ref.path}`);
    for (const item of snapshot.items) this.#output.appendLine(this.#item(item));
    for (const waiting of snapshot.interrupts)
      this.#output.appendLine(
        `Run ${waiting.rootRunId} is waiting for ${waiting.interrupts.length} response(s). Use Flame: Respond to Waiting Run.`,
      );
  }

  #item(item: Item): string {
    switch (item.type) {
      case "userMessage":
      case "agentMessage":
        return `${item.type === "userMessage" ? "You" : "Flame"}: ${(item.content ?? []).map((block) => (block.type === "text" ? block.text : "[image]")).join("\n")}`;
      case "reasoning":
        return item.redacted ? "[reasoning redacted]" : (item.text ?? "");
      case "question":
        return item.question.fields.map((field) => field.prompt).join("\n");
      case "toolCall":
        return `${item.tool.name} [${item.status}]\n${JSON.stringify(item.tool.result ?? item.tool.arguments, null, 2)}`;
      case "compaction":
        return `[context compacted]\n${item.summary}`;
    }
  }

  async #showDocument(name: string, text: string, language?: string): Promise<void> {
    let document = await vscode.workspace.openTextDocument(this.#documents.create(name, text));
    if (language) document = await vscode.languages.setTextDocumentLanguage(document, language);
    await vscode.window.showTextDocument(document, { preview: true });
  }

  async #reviewChanges(): Promise<void> {
    const connection = this.#connected();
    const session = this.#selected();
    const diff = await connection.client
      .workspace(session.workspace.ref)
      .diff.get({ format: "raw" }, connection.signal);
    if (this.#connection !== connection) return;
    await this.#showDocument("runtime-changes.diff", diff.patch ?? "", "diff");
    if (diff.truncated)
      await vscode.window.showWarningMessage(
        "Runtime returned a truncated diff; review individual Runtime files for complete content.",
      );
  }

  async #openRuntimeFile(): Promise<void> {
    const connection = this.#connected();
    const session = this.#selected();
    const path = await vscode.window.showInputBox({
      title: "Open Runtime file",
      prompt: "Path relative to the selected Runtime workspace",
      ignoreFocusOut: true,
    });
    if (!path?.trim() || this.#connection !== connection) return;
    const file = await connection.client
      .workspace(session.workspace.ref)
      .files.read({ path: path.trim() }, connection.signal);
    if (this.#connection !== connection) return;
    await this.#showDocument(path.trim(), file.content);
    if (file.truncated)
      await vscode.window.showWarningMessage("Runtime returned a truncated file preview.");
  }

  async #compareContext(): Promise<void> {
    const snapshot = this.#submitted;
    if (!snapshot) throw new Error("send an editor selection first");
    const document = await vscode.workspace.openTextDocument(vscode.Uri.parse(snapshot.uri));
    const before = this.#documents.create("submitted-buffer", snapshot.text);
    const after = this.#documents.create("current-buffer", document.getText());
    await vscode.commands.executeCommand(
      "vscode.diff",
      before,
      after,
      `Submitted v${snapshot.documentVersion} ↔ Editor v${document.version}`,
      { preview: true },
    );
  }

  async close(): Promise<void> {
    if (this.#closed) return;
    this.#closed = true;
    await this.#disconnect();
    await Promise.allSettled([...this.#tasks, this.#refreshing]);
    for (const disposable of this.#disposables.reverse()) disposable.dispose();
  }
}

let workbench: Workbench | undefined;

export function activate(context: vscode.ExtensionContext): void {
  workbench = new Workbench(context);
}

export async function deactivate(): Promise<void> {
  await workbench?.close();
  workbench = undefined;
}
