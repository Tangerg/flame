# Code Mode

Code Mode lets the model batch reads through a short program instead of
returning every intermediate page to its context. The built-in `run_script`
Tool runs JavaScript that calls the Run's read-only Tools, pages, filters and
joins their results, and returns only its conclusion. Simple queries keep
calling Tools directly.

## Owners

Runtime owns `run_script`. Every call a script makes is an ordinary Tool call
of the same Run: lifecycle hooks, argument validation, approval policy and the
Tool's own execution apply exactly as for a direct call. Only Runtime can
apply those checks, so a plugin cannot provide this Tool.

A Tool's own contract owns whether it is read-only. A script may call a Tool
whose definition declares Scope's safe effect, which promises no mutation of
application state. Built-in `read`, `glob` and `grep` declare it. An installed
package declares it through the MCP read-only annotation, which Runtime trusts
only for installed sources; a user's MCP server stays untrusted. Any other Tool,
including `run_script` itself, is absent from scripts.

A script declares the Tools it calls, and it can call only those. Declaring a
Tool that is not read-only refuses the whole call before it runs.

The script engine is a QuickJS virtual machine with no filesystem, network,
environment, timers or module loading. The only host functions are the declared
Tools and `describeTool`. JavaScript numbers are float64, so a result number that
a float64 cannot reproduce reaches the script as its decimal text instead of
being rounded.

## Approval

A running script cannot pause for an answer, so it asks before it runs. When a
declared Tool's own calls would ask the user under the Session's mode and
remembered rules, the `run_script` call asks once, showing its source and Tools.
Approving it admits those Tools' approval prompts for that script only.
Remembering the answer keys a rule on exactly that set of Tools. Plan mode,
remembered denials and hook refusals still reject individual calls, and a call
that a hook asks about is refused because the user did not see its arguments.
When a script was admitted without asking, a declared Tool that later needs
approval, for example after its rule was removed, is refused.

## Calls

A script call runs under the Run's current admission. A remembered denial or a
hook refusal reaches the script as a rejected promise. Reads made by a script do
not grant read-before-write authority: the model has not seen what the script
read.

Each call that actually ran is recorded with its source, effective arguments and
status: succeeded, failed or rejected. A call still in flight when the script
ends is unconfirmed; a call the script requested but Runtime never started is
not dispatched. Local computation records nothing. The record keeps the exact
result of every call the script received, so a client can check each conclusion
against the data it came from. A succeeded call whose result the script never
received records only its size.

## Limits

A script has a wall-clock deadline, a memory limit, a maximum number of calls,
a bound on concurrent calls, a bound on the tool results it receives and a bound
on its output. The received-results bound also bounds the record. Exceeding the
deadline, call count, received results or output ends the script with that
reason; the engine reports memory exhaustion as its own error. Calls in flight
are canceled and recorded as unconfirmed. A promise that can never settle ends
the script immediately. Canceling one script alone is not offered: cancellation
is a Run's, and a canceled Run records the script's calls as above.

The Tool result gives the model the script's output and a count of call
outcomes. The structured result carries the script, its status and the call
record for clients.

## Recovery

`run_script` declares the safe effect: it can only read. If Runtime stops
during a script, recovery may run the script again under the same call identity
and current admission, which may ask again. A script never resumes a paused call stack, and writes
remain direct Tool calls with their own approval and receipts.
