import { mkdir, mkdtemp, rm, stat } from "node:fs/promises";
import { userInfo } from "node:os";
import { join } from "node:path";
import { randomUUID } from "node:crypto";
import {
  Engine,
  EngineStartupFailure,
  type EngineCallScope,
  type EngineConfig,
  type EngineHooks,
  type EngineResult,
  type EngineSurface,
} from "./engine";
import type { Configuration } from "./configuration";
import { Installation, InstallationError } from "./installation";
import { validateBrowser } from "./browser";
import { prepareSandbox } from "./sandboxLaunch";

function childEnvironment(
  config: Configuration,
  application: Installation,
  control: string,
): Record<string, string> {
  const account = userInfo();
  if (
    [application.modules, application.resources, config.packageRoot].some(
      (path) => path.includes(":"),
    )
  )
    throw new InstallationError(
      "CUA dependency paths cannot contain the engine's path-list separator",
    );
  const environment: Record<string, string> = {
    ...config.environment,
    HOME: account.homedir,
    USER: account.username,
    LOGNAME: account.username,
    PATH: `${join(application.runtime, "bin")}:/usr/bin:/bin:/usr/sbin:/sbin`,
    TMPDIR: join(control, "tmp"),
    CODEX_HOME: join(account.homedir, ".codex"),
    CODEX_CLI_PATH: application.codex,
    CUA_REPL_NODE_REPL_PATH: application.nodeRepl,
    NODE_REPL_NODE_PATH: application.node,
    NODE_REPL_NODE_MODULE_DIRS: application.modules,
    NODE_REPL_TRUSTED_CODE_PATHS: [
      application.modules,
      join(application.resources, "plugins"),
      ...(application.native
        ? [join(config.packageRoot, "io.github.tangerg.flame/engine")]
        : []),
    ].join(":"),
    CUA_REPL_ENABLED_SURFACES: config.browser ? "browser,computer" : "computer",
    CUA_REPL_BROWSER_ENV: "codex-app",
    NODE_REPL_DISABLE_ANALYTICS: "1",
    NODE_REPL_REQUEST_META: JSON.stringify({
      "x-codex-turn-metadata": {
        session_id: `flame-computer-${randomUUID()}`,
        turn_id: "initialization",
      },
    }),
  };
  if (config.browser)
    Object.assign(environment, {
      BROWSER_USE_AVAILABLE_BACKENDS: "chrome",
      BROWSER_USE_TINYSKY_ENABLED: "1",
      BROWSER_USE_CODEX_APP_BUILD_FLAVOR: "prod",
      BROWSER_USE_CODEX_APP_VERSION: application.version,
    });
  if (config.audio)
    Object.assign(environment, {
      SKY_ENABLE_AUDIO: "1",
      NODE_REPL_ENABLE_AUDIO: "1",
    });
  if (application.native)
    environment.SKY_CUA_SERVICE_PATH = application.native.bundle;
  return environment;
}

/** Owns one captured application generation and its private execution directories. */
export class Cua {
  readonly #installation: Installation;
  readonly #engine: Engine;
  readonly #hooks: EngineHooks;
  readonly #directory: string;
  #closing?: Promise<void>;

  private constructor(
    installation: Installation,
    engine: Engine,
    hooks: EngineHooks,
    directory: string,
  ) {
    this.#installation = installation;
    this.#engine = engine;
    this.#hooks = hooks;
    this.#directory = directory;
  }

  static async connect(
    config: Configuration,
    hooks: EngineHooks,
  ): Promise<Cua> {
    let directory: string | undefined;
    let engine: Engine | undefined;
    try {
      hooks.signal.throwIfAborted();
      const installation = await Installation.inspect(
        config.application,
        hooks.signal,
      );
      if (
        installation.platform === "linux" &&
        (!config.environment.DISPLAY ||
          !config.environment.DBUS_SESSION_BUS_ADDRESS ||
          config.environment.XDG_SESSION_TYPE === "wayland")
      )
        throw new InstallationError(
          "Linux CUA requires an explicitly selected X11 desktop and D-Bus session",
        );
      const leases = join(config.data, "leases");
      await mkdir(leases, { recursive: true, mode: 0o700 });
      directory = await mkdtemp(join(leases, "lease-"));
      const controlDirectory = join(directory, "control");
      const workdir = join(directory, "work");
      await mkdir(join(controlDirectory, "tmp"), {
        recursive: true,
        mode: 0o700,
      });
      await mkdir(workdir, { mode: 0o700 });
      const environment = childEnvironment(
        config,
        installation,
        controlDirectory,
      );
      try {
        if (!(await stat(environment.CODEX_HOME!)).isDirectory())
          throw new Error("engine account state is not a directory");
      } catch (cause) {
        throw new InstallationError(
          "initialize the selected CUA application for this account before opening computer control",
          cause,
        );
      }
      if (config.browser)
        await validateBrowser(installation, environment, hooks.signal);
      const prepared = await prepareSandbox(
        {
          runtime: installation.runtime,
          node: installation.node,
          codex: installation.codex,
          packageRoot: config.packageRoot,
          workdir,
          controlDirectory,
          environment,
        },
        hooks.signal,
      );
      Object.assign(environment, prepared.environment);
      environment.NODE_REPL_REQUEST_META = JSON.stringify({
        ...JSON.parse(environment.NODE_REPL_REQUEST_META!),
        "codex/sandbox-state-meta": prepared.sandbox,
      });
      const launch: EngineConfig = {
        command: installation.node,
        args: [installation.entry],
        env: environment,
        ownershipPath: join(config.data, "engine-ownership.json"),
        cwd: workdir,
      };
      if (installation.native) {
        launch.native = {
          service: installation.native.service,
          client: installation.native.client,
          helper: installation.native.helper,
          wrapper: join(
            config.packageRoot,
            "io.github.tangerg.flame/engine/native-sky.mjs",
          ),
        };
      }
      if (launch.native)
        environment.NODE_REPL_TRUSTED_SERVICES = JSON.stringify({
          sky: launch.native.wrapper,
          ...(config.browser
            ? { browser: "@oai/browser-desktop/service" }
            : {}),
        });
      hooks.signal.throwIfAborted();
      installation.assertCurrent();
      engine = await Engine.connect(launch, hooks);
      installation.assertCurrent();
      return new Cua(installation, engine, hooks, directory);
    } catch (cause) {
      if (engine) {
        try {
          await engine.close();
        } catch (cleanup) {
          throw new EngineStartupFailure(
            "unconfirmed",
            new AggregateError([cause, cleanup]),
          );
        }
      }
      if (
        cause instanceof EngineStartupFailure &&
        cause.cleanup === "unconfirmed"
      )
        throw cause;
      if (directory) await rm(directory, { recursive: true, force: true });
      if (cause instanceof EngineStartupFailure) throw cause;
      throw new EngineStartupFailure(
        "closed",
        cause,
        cause instanceof InstallationError
          ? [cause.message]
          : ["CUA dependency preparation failed"],
      );
    }
  }

  surface(): Readonly<EngineSurface> {
    return this.#engine.surface();
  }

  async call(
    tool: "js" | "js_reset",
    args: Record<string, unknown>,
    scope: EngineCallScope,
  ): Promise<EngineResult> {
    try {
      this.#installation.assertCurrent();
    } catch (cause) {
      this.#hooks.onRetired(cause as Error);
      await this.#engine.close();
      throw cause;
    }
    const outcome = await this.#engine.call(tool, args, scope);
    try {
      this.#installation.assertCurrent();
    } catch (cause) {
      this.#hooks.onRetired(cause as Error);
      await this.#engine.close().catch(() => {});
      return { result: outcome.result, cleanup: "unconfirmed" };
    }
    return outcome;
  }

  close(): Promise<void> {
    this.#closing ??= Promise.resolve().then(async () => {
      await this.#engine.close();
      await rm(this.#directory, { recursive: true, force: true });
    });
    return this.#closing;
  }
}
