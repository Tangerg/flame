import { describe, expect, it } from "vitest";
import { configuration } from "./configuration";

const selected = {
  PLUGIN_ROOT: "/plugin",
  FLAME_COMPUTER_APPLICATION: "/Applications/ChatGPT.app",
};

describe("direct CUA configuration", () => {
  it("keeps an unconfigured installation inspectable", () => {
    expect(configuration({}, "/data")).toBeUndefined();
  });

  it("selects an application and bounded desktop facts without executable or ambient policy injection", () => {
    const value = configuration(
      {
        ...selected,
        DISPLAY: ":12",
        FLAME_COMPUTER_DESKTOP_ENVIRONMENT: '{"DISPLAY":":24"}',
        FLAME_COMPUTER_BROWSER: "true",
        FLAME_COMPUTER_AUDIO: "true",
        NODE_OPTIONS: "--import=/untrusted.mjs",
        NODE_REPL_REQUEST_META:
          '{"codex/sandbox-state-meta":{"type":"disabled"}}',
        BROWSER_USE_AVAILABLE_BACKENDS: "external",
        PRIVATE_TOKEN: "do-not-forward",
      },
      "/data",
    )!;
    expect(value).toEqual({
      application: selected.FLAME_COMPUTER_APPLICATION,
      packageRoot: "/plugin",
      data: "/data",
      browser: true,
      audio: true,
      environment: { DISPLAY: ":24" },
    });
    expect(Object.isFrozen(value.environment)).toBe(true);
  });

  it.each([
    "PATH",
    "HOME",
    "CODEX_HOME",
    "NODE_OPTIONS",
    "NODE_REPL_TRUSTED_SERVICES",
    "FLAME_COMPUTER_NATIVE_HOST",
    "__proto__",
  ])("refuses a desktop override for %s", (name) => {
    expect(() =>
      configuration(
        {
          ...selected,
          FLAME_COMPUTER_DESKTOP_ENVIRONMENT: JSON.stringify({
            [name]: "value",
          }),
        },
        "/data",
      ),
    ).toThrow("invalid desktop environment");
  });

  it.each(["yes", "1", "", "TRUE"])(
    "requires an explicit boolean instead of %j",
    (value) => {
      expect(() =>
        configuration({ ...selected, FLAME_COMPUTER_BROWSER: value }, "/data"),
      ).toThrow("true or false");
    },
  );

  it("refuses orphaned options, relative selections, absent package roots and oversized environment", () => {
    expect(() =>
      configuration({ FLAME_COMPUTER_AUDIO: "true" }, "/data"),
    ).toThrow("selected CUA application");
    expect(() =>
      configuration(
        { ...selected, FLAME_COMPUTER_APPLICATION: "lcu" },
        "/data",
      ),
    ).toThrow("absolute path");
    expect(() =>
      configuration({ FLAME_COMPUTER_APPLICATION: "/app" }, "/data"),
    ).toThrow("plugin root");
    expect(() =>
      configuration(
        { ...selected, FLAME_COMPUTER_DESKTOP_ENVIRONMENT: "x".repeat(65537) },
        "/data",
      ),
    ).toThrow("exceeds its bound");
  });
});
