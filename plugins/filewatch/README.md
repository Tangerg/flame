# File watch

This optional backend owns one persistent trigger per installation, its revision,
accepted observations and immutable dispatch intent. It has no component or model
Tool. Runtime owns path admission, OS observation, bounded subscription delivery
and canonical Session/Run acceptance, execution and accounting.

Build `go build -o plugins/filewatch/bin/filewatch ./plugins/filewatch/backend`,
then install the package and enable its declared background contribution through
the ordinary plugin lifecycle. The public generated contract exports `get`,
`configure`, `status`, `events` and `event` under `flame.filewatch.v1`. Regenerate with
`go generate ./plugins/filewatch/internal/contract`.

`configure` replaces the inspected revision with one trigger; omitting `trigger`
disables observation. Use an available canonical workspace returned by
`workspaces.resolve`, 1–32 exact workspace-relative paths, a title, instructions
and an exact Provider/Model selection. Reasoning effort belongs to that selection.
Paths cannot escape the workspace through traversal or symlinks. Model construction
and availability checks remain in Runtime. Changing configuration, including an
identical rearm, advances its revision. Configuration acceptance is a desired rule;
`status` reports the actual observed revision, phase and errors.

The activation uses the existing public `runtime.subscribe` file-watch mechanism.
There is no new callback Tool, webhook listener, private fsnotify loop or event bus.
Files observe bounded content, then metadata above Runtime's byte limit;
directories observe immediate entry metadata, never recursive contents. Missing
targets remain observed through their nearest existing parent. OS notifications
may coalesce. Only notifications attributed to this watch can originate ordinary
observations. Git invalidation, subscription resync or a sequence gap instead
captures a `resync` observation requesting inspection of every configured path.
It never claims a specific write happened. The original instructions and a JSON
observation block become the captured Run input; file contents are not copied.

An initialization or streaming failure stops that watch and remains visible until
an explicit configuration rearm. No blind reconnect hides a coverage gap. Disable,
replacement and shutdown cancel and join the subscription. Startup establishes a
new observation baseline; changes while no subscription existed are not replayed.
This is an active observation source, not a durable write audit or exactly-once
filesystem event service. Triggered agents can write watched paths and cause more
observations; select source paths and instructions with that feedback in mind.

The backend holds an exclusive `PLUGIN_DATA` lease and opens `filewatch.db` only.
Configuration commands and their standard SDK receipts commit atomically. Before
any Run call, a private transaction retains the original observation generation,
sequence, definition revision, instructions, exact model, paths, timestamp,
Session/Run identities and automatic installation invocation. Later configuration
changes never rewrite captured work. The private store is bound to one Runtime
idempotency namespace and refuses another store.

Dispatch always queries `runs.getAcceptance` with that exact original opening and
key before `runs.start`. A lost response or private commit leaves the intent pending;
the activation reconciles it through the same public owner after restart. Accepted
or retired core acceptance settles the private intent, including after Run deletion.
Detaching the returned execution stream does not cancel the independently accepted
Run. The private `accepted` flag means the opening was confirmed, not execution
completion. No Run lifecycle or accounting facts live in this plugin.

At most 32 pending intents and 256 total observations are retained. New observations
prune only the oldest confirmed historical rows. Pending rows are never removed.
`events` returns compact headers; `event` reads one full immutable capture so large
instruction/path archives do not exceed the callback's response frame budget.
Capacity exhaustion stops observation explicitly; pending reconciliation continues.
The one-second wake checks configuration and pending intent, not the filesystem.
Accepted intents continue reconciliation even after the rule is disabled. An
unavailable model or refused Run opening stays pending with `dispatchError` rather
than changing its selection or inventing success. Uninstalling a package removes
its activation; preserve its private data to retain pending work.
