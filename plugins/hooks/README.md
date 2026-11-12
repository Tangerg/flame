# Installed lifecycle hooks

This optional backend implements `flame.hooks.v1`. It reads global
`<home>/.flame/hooks.json` and the project cascade between the Runtime's captured
project root and working directory. Runtime supplies that identity and decides
whether project configuration may be read. Untrusted project files cannot break
otherwise valid global hooks; management inspection can still validate them.

The backend owns bounded shell execution, process-group cancellation, output
parsing and original command receipts. An admitted command with no known receipt
cannot execute again under that identity. Known results survive a backend restart.
Commands run in the installed process environment, including its launch PATH;
other ambient Runtime variables are not inherited. Set required variables
explicitly in the trusted authored command.

Runtime owns project trust, event delivery, bounded material, Tool argument
admission and human approval. The captured provider orders and matches its rules,
executes each admitted shell effect at most once, and combines the outcomes into
one event receipt. Direct public plugin calls cannot originate trusted lifecycle events. `allow` advice cannot override those guarantees.
Observe-only events cannot change an already committed outcome. A captured rule
set retains its exact source until all of its events finish and close.

Command rules may set `"failurePolicy": "required"` for `PreToolUse`,
`UserPromptSubmit`, or `PreCompact`. A timeout, unavailable runner, invalid input
projection, malformed decision, or execution failure then denies the pending
action and retains the original diagnostic. An omitted policy or
`"failurePolicy": "advisory"` reports the failure and continues. A valid explicit
deny or approval request keeps its meaning in both modes. Required policies are
rejected on declarative injections and events that cannot veto their pending
action; they cannot undo an effect from an observe-only event. The policy is part
of the captured rule, so changing the authored file does not change an admitted
event's interpretation.

Fresh Runtime catalogs select this backend. Existing installations must install,
enable and select its `hook-provider` contribution for `flame.hooks.v1`. There is
no built-in discovery or shell fallback. Disabling or uninstalling the backend
withdraws future rules; project trust and historical facts remain in Runtime.
Accepted commands retain their original routing and receipt owner. Finish pending
work with its matching release before replacing it.

The optional settings component inspects rules through public workspace reads and
changes project trust through Runtime. It retains the last accepted catalog during
read failures, and an uncertain trust command must be reconciled using its original
mutation before another intent is admitted. Source retirement cancels observation;
it does not infer a command's outcome.

Display-language changes preserve the accepted catalog, read failure and original
pending read without restarting observation. Reads and watches follow the captured
Runtime, tracking function, cancellation signal and workspace target. Explicit
Refresh/Retry, subscription establishment and matching Runtime change/resync events
still request fresh reads; actual source or target retirement still cancels them.

Keyboard focus on a fitting project-trust row reveals its shell-execution
consequence and captured project root through the shared Panel owner; an
oversized row falls back to revealing the focused control.
