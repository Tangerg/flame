const minimumMacOSVersion = "13.0";
const deploymentFlag = `-mmacosx-version-min=${minimumMacOSVersion}`;
const externalLinkerFlags = [
  deploymentFlag,
  // Go adds libobjc per Objective-C package; remove this when Go #67799 is fixed.
  "-Wl,-no_warn_duplicate_libraries",
  "-Wl,-fatal_warnings",
];

export const desktopProduct = Object.freeze({
  identifier: "com.tangerg.flame",
  version: "0.0.1",
  description: "Agent client for the Flame Runtime",
  copyright: "(c) 2026, tangerg",
  macOS: Object.freeze({
    minimumVersion: minimumMacOSVersion,
    compilerFlags: `-O2 -g ${deploymentFlag}`,
    linkerFlags: `-macos=${minimumMacOSVersion} -extldflags '${externalLinkerFlags.join(" ")}'`,
  }),
});
