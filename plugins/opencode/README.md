# OpenCode sessions

Enable the OpenCode plugin and supply the absolute OpenCode executable in its
installation settings when the Runtime process cannot find it on its PATH.
OpenCode must already have access to its provider on the Runtime machine. Open
the workspace view, list sessions for an absolute working directory, and select
an existing native session. The view shows that session's native ID, machine,
working directory and agent version.

The connector starts the executable with `acp` only when a user lists or opens
sessions. It sends and receives Agent Client Protocol v1 over the child
process's stdio. It loads the original history and keeps prompt, permission and
cancellation records in private plugin storage. Only a terminal stop reason
from the original prompt confirms cancellation. If the process is lost, the
original permission and requested cancellation stay visible as unconfirmed.
Reconnect loads the same stored native session. The user may explicitly start
a fresh turn after inspecting an unconfirmed result.

Fork is available when the connected OpenCode advertises it. Diff reads use
the Runtime's read-only Git projection of the exact workspace. These reads
include pending workspace changes. The OpenCode process uses its own
local configuration and permissions.

To exercise the installed OpenCode with a temporary workspace and a loopback
model fixture, set `FLAME_OPENCODE_NATIVE_EXECUTABLE` to its absolute executable
and run `go test ./runtime/internal/bootstrap -run '^TestOpenCodeHost'` from
the repository workspace.

[OpenCode ACP documentation](https://opencode.ai/docs/cli/acp/)
