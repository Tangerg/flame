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

Runtime owns project trust, event order, final decision folding, Tool argument
admission and human approval. `allow` advice cannot override those guarantees.
Observe-only events cannot change an already committed outcome. A captured rule
set retains its exact source until all of its events finish and close.

Fresh Runtime catalogs select this backend. Existing installations must install,
enable and select its `hook-provider` contribution for `flame.hooks.v1`. There is
no built-in discovery or shell fallback. Disabling or uninstalling the backend
withdraws future rules; project trust and historical facts remain in Runtime.
Accepted commands retain their original routing and receipt owner. Finish pending
work with its matching release before replacing it.
