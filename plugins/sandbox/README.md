# Native sandbox

`flame.sandbox` executes a bounded `/bin/sh` script in a fresh private scratch
workspace through the existing native confinement mechanism. The typed `execute`
command and `sandbox_execute` MCP Tool share one execution/admission owner.

`limits` publishes actual platform availability and bounds. Today the native
backend is macOS Seatbelt; other platforms refuse explicitly. Writes are confined
to the scratch directory, the core-configured home is hidden, networking is denied
and inherited credentials are removed from the child environment. Reads outside
the hidden home follow the existing native policy; this is not a hermetic guest
filesystem. No CPU, memory or process-count quota is promised. Docker is not a
dependency, and core Run execution, model construction and accounting are unchanged.

Scripts are nonblank UTF-8 without NUL, at most 64 KiB, with an explicit timeout
between 1 and 30,000 ms. Each stream retains at most the native runner's 256 KiB
capture plus its truncation marker. Valid UTF-8 output remains text; other bytes
are encoded as base64 with an explicit encoding tag. Nonzero exit status is a
completed execution fact. A native execution that observes its time limit reports
`timed_out`; cancellation kills the command group through the shared runner.
Normal cleanup joins execution before removing the scratch workspace.

The plugin owns original input and outcomes in `PLUGIN_DATA/sandbox.db`. It
commits command admission before acquiring the call-scoped core discovery callback
or starting a process. Discovery supplies the actual configured home; neither
model arguments nor the backend's scrubbed parent environment invent that path.
Only a newly admitted command may execute. Result/refusal settlement is atomic;
replay returns the original result, including its original duration. Missing
settlement remains unknown after interruption/restart and never authorizes a
second process. A crash may leave private scratch material and an unconfirmed
command; it is not reported as a completed or safely cancelled execution.

Typed and model receipts read that same private outcome. The model receipt only
projects the original result through the Tool codec; it never executes code.

Regenerate the contract with:

```sh
go generate ./runtime/internal/plugins/sandbox/rpc
```

Build the ordinary target package with `plugins/build.mjs`; installation never
compiles source. The backend uses no shared UI components or new dependencies.
