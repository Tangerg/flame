import { afterEach, expect, it, vi } from "vitest";
import type { PluginInstallation, Session } from "@flame/runtime-contract/wire";
import sessionFixture from "@flame/runtime-contract/samples/session.json";
import type { CommandResult, Connection } from "./connection";
import { runPluginAction } from "./pluginActions";

const savedSession = sessionFixture as Session;

interface NativeInput {
  title: string;
  prompt: string;
  value: string;
  busy: boolean;
  enabled: boolean;
  validationMessage?: string;
  disposed: boolean;
  accept(): void;
  change(value: string): void;
  hide(): void;
}

const native = vi.hoisted(() => ({ inputs: [] as NativeInput[], pick: vi.fn() }));

vi.mock("vscode", () => ({
  CancellationTokenSource: class {
    #listeners = new Set<() => void>();
    token = {
      isCancellationRequested: false,
      onCancellationRequested: (listener: () => void) => {
        this.#listeners.add(listener);
        return { dispose: () => this.#listeners.delete(listener) };
      },
    };
    cancel() {
      this.token.isCancellationRequested = true;
      for (const listener of this.#listeners) listener();
    }
    dispose() {
      this.#listeners.clear();
    }
  },
  window: {
    showQuickPick: native.pick,
    createInputBox: () => {
      const accepted = new Set<() => void>();
      const hidden = new Set<() => void>();
      const changed = new Set<() => void>();
      const subscribe = (listeners: Set<() => void>) => (listener: () => void) => {
        listeners.add(listener);
        return { dispose: () => listeners.delete(listener) };
      };
      const input = {
        title: "",
        prompt: "",
        value: "",
        busy: false,
        enabled: true,
        disposed: false,
        validationMessage: undefined as string | undefined,
        onDidAccept: subscribe(accepted),
        onDidHide: subscribe(hidden),
        onDidChangeValue: subscribe(changed),
        show: vi.fn(),
        accept() {
          for (const listener of accepted) listener();
        },
        hide() {
          for (const listener of hidden) listener();
        },
        change(value: string) {
          this.value = value;
          for (const listener of changed) listener();
        },
        dispose() {
          this.disposed = true;
        },
      };
      native.inputs.push(input);
      return input;
    },
  },
}));

afterEach(() => {
  native.inputs.length = 0;
  vi.clearAllMocks();
});

function fixture(
  execute = vi.fn<Connection["execute"]>(async () => ({ sessionResult: savedSession })),
) {
  const lifetime = new AbortController();
  const session: Session = {
    ...structuredClone(savedSession),
    id: "ses_original",
    title: "Original",
    revision: 7,
  };
  const installation: PluginInstallation = {
    id: "940ac827-b431-455b-af4b-e3a170bcfda0",
    source: "/package",
    state: "enabled",
    presentation: "admitted",
    realization: { type: "available" },
    inputStates: {},
    disabledServers: [],
    disabledSkills: [],
    selected: {
      digest: "1".repeat(64),
      name: "Review package",
      actions: [{ id: "rename", title: "Name investigation", operation: "renameSession" }],
      servers: [],
      inputs: [],
      themes: [],
      views: [],
      skills: [],
      diagnostics: [],
    },
  };
  const list = vi.fn(async () => ({ data: [installation] }));
  const connection = {
    signal: lifetime.signal,
    client: { plugins: { list } },
    execute,
  } as unknown as Connection;
  native.pick.mockImplementation(async (choices) => choices[0]);
  return { lifetime, session, installation, execute, list, connection };
}

async function form() {
  await vi.waitFor(() => expect(native.inputs).toHaveLength(1));
  return native.inputs[0]!;
}

it("captures the Session and declared release before explicit acceptance", async () => {
  const f = fixture();
  const result = runPluginAction(f.connection, f.session);
  f.session.id = "ses_successor";
  f.session.revision = 12;
  const input = await form();
  expect(f.execute).not.toHaveBeenCalled();
  expect(input.prompt).toContain("ses_original");
  f.installation.selected.digest = "2".repeat(64);
  input.change("Reviewed");
  input.accept();
  await expect(result).resolves.toEqual({ sessionResult: sessionFixture });
  expect(f.execute).toHaveBeenCalledExactlyOnceWith({
    method: "plugins.renameSession",
    params: {
      installationId: f.installation.id,
      digest: "1".repeat(64),
      actionId: "rename",
      update: { sessionId: "ses_original", expectedRevision: 7, title: "Reviewed" },
    },
  });
  expect(input.disposed).toBe(true);
});

it("preserves a refused title and inspected revision until the user closes the form", async () => {
  const f = fixture(
    vi.fn(async () => {
      throw new Error("revision conflict");
    }),
  );
  const result = runPluginAction(f.connection, f.session);
  const input = await form();
  input.change("Reviewed");
  input.accept();
  await vi.waitFor(() => expect(input.validationMessage).toBe("revision conflict"));
  f.session.revision = 8;
  expect(input.value).toBe("Reviewed");
  expect(input.enabled).toBe(true);
  input.change("Another title");
  input.accept();
  await vi.waitFor(() => expect(f.execute).toHaveBeenCalledTimes(2));
  for (const [command] of f.execute.mock.calls) {
    expect(command.params).toMatchObject({
      update: { sessionId: "ses_original", expectedRevision: 7 },
    });
  }
  input.hide();
  await expect(result).resolves.toBeUndefined();
});

it("closing after acceptance retires presentation without cancelling or republishing the result", async () => {
  let complete!: (value: CommandResult) => void;
  const f = fixture(
    vi.fn(
      () =>
        new Promise<CommandResult>((resolve) => {
          complete = resolve;
        }),
    ),
  );
  const result = runPluginAction(f.connection, f.session);
  const input = await form();
  input.accept();
  input.accept();
  expect(f.execute).toHaveBeenCalledOnce();
  input.hide();
  await expect(result).resolves.toBeUndefined();
  expect(f.lifetime.signal.aborted).toBe(false);
  complete({ sessionResult: savedSession });
  await Promise.resolve();
  expect(input.validationMessage).toBeUndefined();
  expect(f.execute).toHaveBeenCalledOnce();
});

it("connection retirement disposes the form and prevents a late acceptance", async () => {
  const f = fixture();
  const result = runPluginAction(f.connection, f.session);
  const input = await form();
  f.lifetime.abort();
  await expect(result).resolves.toBeUndefined();
  expect(input.disposed).toBe(true);
  input.accept();
  expect(f.execute).not.toHaveBeenCalled();
});

it("a picker retiring during selection cannot open or submit a successor form", async () => {
  const f = fixture();
  let choose!: () => void;
  native.pick.mockImplementation(
    (choices) =>
      new Promise((resolve) => {
        choose = () => resolve(choices[0]);
      }),
  );
  const result = runPluginAction(f.connection, f.session);
  await vi.waitFor(() => expect(native.pick).toHaveBeenCalledOnce());
  f.lifetime.abort();
  choose();
  await expect(result).resolves.toBeUndefined();
  expect(native.inputs).toHaveLength(0);
  expect(f.execute).not.toHaveBeenCalled();
});

it("unapproved or withdrawn presentation cannot originate an action", async () => {
  for (const state of ["unapproved", "withdrawn"] as const) {
    const f = fixture();
    if (state === "unapproved") f.installation.state = "unapproved";
    else f.installation.presentation = "withheld";
    await expect(runPluginAction(f.connection, f.session)).rejects.toThrow("no enabled plugin");
    expect(f.execute).not.toHaveBeenCalled();
    expect(native.inputs).toHaveLength(0);
  }
});
