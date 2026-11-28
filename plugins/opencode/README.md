# ACP sessions

Enable the OpenCode plugin to open the **ACP sessions** workspace view. The
OpenCode profile uses the absolute executable in the plugin settings or the
OpenCode on the Runtime's PATH. Each agent uses its own provider configuration
on the Runtime machine.

For another ACP agent, install its executable on the Runtime machine and set
the plugin's optional `agents` input. For example, install the pinned Pi
adapter and Pi on the machine, then use the actual absolute executable path.

```sh
npm install -g --save-exact pi-acp@0.0.35 @earendil-works/pi-coding-agent@0.81.0
```

```json
[
  {
    "id": "pi",
    "name": "Pi",
    "executable": "/path/to/pi-acp",
    "nativeName": "pi-acp",
    "registry": "pi-acp",
    "version": "0.0.35"
  }
]
```

Additional profiles can include an `args` array. The Registry version and
native name bind the installed program to its initialize response. Flame
validates the executable and starts it with the approved arguments directly.
A profile and physical working directory identify each live connection. The
original program, arguments, workspace and native identifier identify its
stored Session. A program or profile change requires the original approved
installation to reconnect a captured Session. [ACP Registry](https://agentclientprotocol.com/get-started/registry)

Pick an agent and workspace, then create a native Session or open an existing
one from its list. Agents without listing can accept an original ID and
workspace you supply. The view displays the native ID, machine, working
directory and agent version. The connector negotiates ACP v1 over stdio
on the first action. It uses `session/load` for replay when advertised and
`session/resume` to reopen an agent that offers resume alone. It keeps prompt, permission and
cancellation records in private plugin storage. Only a terminal stop reason
from the original prompt confirms cancellation. If the process is lost, the
original permission and requested cancellation stay visible as unconfirmed.
Reconnect selects the same stored native Session. The user may explicitly
start a fresh turn after inspecting an unconfirmed result.

Fork is available when the connected OpenCode advertises it. Creation and
fork confirmations retain the native Session and command receipt together.
Open the fork to load its history. Recover a pending command using its original
installation release and contract. Diff reads use the Runtime's read-only
Git projection of the exact workspace, including pending workspace changes.

To exercise the installed OpenCode with a temporary workspace and a loopback
model fixture, set `FLAME_OPENCODE_NATIVE_EXECUTABLE` to its absolute executable
and run `go test ./runtime/internal/bootstrap -run '^TestOpenCodeHost'` from
the repository workspace. Set `FLAME_PI_ACP_NATIVE_EXECUTABLE` and
`FLAME_PI_NATIVE_EXECUTABLE` to exercise the Registry adapter and Pi with
`go test ./runtime/internal/bootstrap -run '^TestACPRegistry'`.

[OpenCode ACP documentation](https://opencode.ai/docs/cli/acp/)
