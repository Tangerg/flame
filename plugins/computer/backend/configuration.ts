import { isAbsolute } from "node:path";
import { z } from "zod";
import { parseJSON } from "@qimu/node";
const desktopVariables = [
  "DISPLAY",
  "XAUTHORITY",
  "DBUS_SESSION_BUS_ADDRESS",
  "XDG_RUNTIME_DIR",
  "XDG_CONFIG_HOME",
  "XDG_DATA_HOME",
  "XDG_SESSION_TYPE",
  "LANG",
  "LC_ALL",
  "LC_CTYPE",
  "TZ",
] as const;
const desktopEnvironment = z.partialRecord(
  z.enum(desktopVariables),
  z
    .string()
    .max(16384)
    .refine((value) => !value.includes("\0")),
);

export interface Configuration {
  readonly application: string;
  readonly packageRoot: string;
  readonly data: string;
  readonly browser: boolean;
  readonly audio: boolean;
  readonly environment: Readonly<Record<string, string>>;
}

function absolute(value: string | undefined, label: string): string {
  if (!value || !isAbsolute(value) || /[\0\r\n]/.test(value))
    throw new Error(`computer: ${label} must be an absolute path`);
  return value;
}

function enabled(value: string | undefined, label: string): boolean {
  if (value === undefined || value === "false") return false;
  if (value === "true") return true;
  throw new Error(`computer: ${label} must be true or false`);
}

export function configuration(
  environment: NodeJS.ProcessEnv,
  data: string,
): Configuration | undefined {
  const application = environment.FLAME_COMPUTER_APPLICATION;
  const browser = enabled(environment.FLAME_COMPUTER_BROWSER, "browser");
  const audio = enabled(environment.FLAME_COMPUTER_AUDIO, "audio");
  if (application === undefined || application === "") {
    if (browser || audio || environment.FLAME_COMPUTER_DESKTOP_ENVIRONMENT)
      throw new Error("computer: options require a selected CUA application");
    return undefined;
  }
  const encoded = environment.FLAME_COMPUTER_DESKTOP_ENVIRONMENT ?? "{}";
  if (Buffer.byteLength(encoded) > 64 * 1024)
    throw new Error("computer: desktop environment exceeds its bound");
  let overrides: Partial<Record<(typeof desktopVariables)[number], string>>;
  try {
    const value = parseJSON(encoded);
    if (
      value &&
      typeof value === "object" &&
      Object.keys(value).some(
        (key) => !(desktopVariables as readonly string[]).includes(key),
      )
    )
      throw new Error("unsupported desktop variable");
    overrides = desktopEnvironment.parse(value);
  } catch (cause) {
    throw new Error("computer: invalid desktop environment", { cause });
  }
  const selected: Record<string, string> = {};
  for (const name of desktopVariables) {
    const value = overrides[name] ?? environment[name];
    if (value !== undefined) {
      if (value.includes("\0") || value.startsWith("()"))
        throw new Error("computer: invalid desktop environment");
      selected[name] = value;
    }
  }
  return Object.freeze({
    application: absolute(application, "CUA application"),
    packageRoot: absolute(environment.PLUGIN_ROOT, "plugin root"),
    data: absolute(data, "plugin data"),
    browser,
    audio,
    environment: Object.freeze(selected),
  });
}
