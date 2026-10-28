import { isAbsolute } from "node:path";
import { pathToFileURL } from "node:url";

interface SandboxPaths {
  readonly workdir: string;
  readonly resources: string;
  readonly guardDirectory: string;
  readonly launcherDirectory: string;
}

export function sandboxState({
  workdir,
  resources,
  guardDirectory,
  launcherDirectory,
}: SandboxPaths) {
  if (
    [workdir, resources, guardDirectory, launcherDirectory].some(
      (path) => !isAbsolute(path) || /[\p{Cc}\p{Surrogate}]/u.test(path),
    ) ||
    new Set([workdir, resources, guardDirectory, launcherDirectory]).size !== 4
  )
    throw new Error("computer: sandbox requires an absolute working directory");
  return {
    permissionProfile: {
      type: "managed" as const,
      file_system: {
        type: "restricted" as const,
        entries: [
          {
            path: { type: "path" as const, path: guardDirectory },
            access: "read" as const,
          },
          {
            path: { type: "path" as const, path: launcherDirectory },
            access: "read" as const,
          },
          {
            path: { type: "path" as const, path: resources },
            access: "read" as const,
          },
          {
            path: { type: "special" as const, value: { kind: "minimal" } },
            access: "read" as const,
          },
          {
            path: { type: "path" as const, path: workdir },
            access: "write" as const,
          },
        ],
      },
      network: "restricted" as const,
    },
    sandboxCwd: pathToFileURL(workdir).href,
  };
}

export function sandboxFilesystem(state: ReturnType<typeof sandboxState>) {
  return Object.fromEntries(
    state.permissionProfile.file_system.entries.map((entry) => [
      entry.path.type === "special"
        ? `:${entry.path.value!.kind}`
        : entry.path.path!,
      entry.access,
    ]),
  );
}
