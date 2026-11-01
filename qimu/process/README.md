# Managed processes

`process.New` combines an independent runner, durable attempt store, executable
source, and a host-selected `Protocol[S]` with a distinct profile identity. The
driver owns process admission, cancellation, closing and recovery. The protocol
binds the approved Unit to a connection function before launch. The function
receives the attached byte stream and returns its typed session and exact Qimu
exports. Return a partially opened session even on setup failure so the driver
can close it.

The optional `qimu/mcp.NewProcessDriver` supplies the MCP handshake, contract
verification, activation and service bindings. A project using a different protocol
can register its own driver under its own profile name, with the same process
lifetime and recovery guarantees.

Supply the same SQLite store and Admission used by the Host. `Source` resolves
the immutable executable. Credentials are supplied to the runner; configuration
identity uses their opaque revision. The caller sets a retirement deadline.

Each creation is protected by a durable attempt before runner dispatch. The
actual process, handshake, setup and exports belong to the instance Lifetime.
Failed setup, stale publication and cancellation close that same resource. A
timeout or lost runner remains unconfirmed and retains its dependency closure.
`Reconcile` consumes late Wait evidence without launching or cleaning up again.

On Host restart, call `Recover` behind the recovery barrier before admitting any
new work. It fences older creation epochs and retires old attempts. An absent
attempt proves non-creation only when the original runner's `ProveAbsent`
observes both the durable fence and missing attempt in one locked operation.
The driver verifies that evidence against the original attempt before settling
its SQLite protection. Changing runners between control requests stays unknown;
a new runner cannot infer ownership from a PID or erase an old protection.

The runner's release proof covers its directly owned child. An application that
allows plugins to start additional processes must provide a stronger execution
profile with evidence for that larger resource set.
