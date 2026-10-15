# MCP startup deadline evidence

This records a failed qualification, not a diagnosed cause or a passing gate.
It is separate from the historical safe-steer post-Resume deadline. No provider
replay, production timeout change, assertion relaxation or UI test is involved.

## Candidate and command

Captured on 2026-10-15 in the existing `codex/unified-plugins` worktree, based on
`523b2e917`, with the test-only fixture/CLI changes identified below. Production
Go is unchanged from `b2dcbfbba`. The command has no overlay or exclusions:

```sh
go test -race -p 2 ./runtime/internal/adapter/run/execution ./runtime/internal/bootstrap -count=1 -timeout=900s -v
```

The 900s value is the package ceiling. Original per-case discovery remains 10s;
the SIGKILL helper/parent limits also remain unchanged. Exit 1, wall 551.760s:

| Package | Duration | Top-level passes | Failures | Skips |
| --- | --- | --- | --- | --- |
| execution | 392.755s | 236 | 1 | 0 |
| bootstrap | 543.824s | 144 | 4 | 0 |

Failures:

- `TestScheduleCreateRecoversAcrossSIGKILLCuts`: edited cannot reach its initial
  durable cut. Helper fails 10.25s; edited subtest fails 10.75s. The other seven
  cuts pass. The parent fails 127.53s.
- `TestMemoryCommittedReplyLossReplaysFromItsSingleOwner`: 13.03s.
- `TestScheduleLostCreateReplyReplaysAfterRevocationAndRestart`: 15.08s.
- `TestScheduleRunNowReplyLossRetainsAcceptanceAfterRestartAndSessionDeletion`: 15.10s.
- `TestScheduleLostRunNowReplyRetainsCapturedWorkOnStartup`: 24.55s.

All five failure snapshots project `{"type":"connecting"}`. Every retained
connection-attempt stack is in SDK `Client.discover`, waiting for the first
`server/discover` response. This happens before Flame's `VerifyCapabilities`,
admission, setup, activation and Core callback connection. The snapshot does not
show whether the backend has started, obtained its directory lease, completed
schema setup, received the frame, or sent a reply through the proxy. Build overlap
and later passing runs do not establish any of those facts or explain this failure.

## Representative live owner stack

This is the edited-cut helper's snapshot, taken only after its original deadline
expires and before cleanup cancels the connection owner. Source prefixes are
normalized to `$WORKTREE` and `$GOMODCACHE`, with indentation normalized; frame
names, offsets and states are
retained. The full snapshot includes the main test, database, transport reader
and connection-attempt owners.

```text
goroutine 37 [running]:
github.com/Tangerg/flame/runtime/internal/bootstrap.(*poMCPRuntime).poMCPTools(0x4bd68804fdf8, 0x4bd688271c80, 0x1095a3bc8)
    $WORKTREE/runtime/internal/bootstrap/plugin_acceptance_mcp_test.go:129 +0x264
github.com/Tangerg/flame/runtime/internal/bootstrap.TestScheduleRecoveryCrashHelper(0x4bd687e69d48)
    $WORKTREE/runtime/internal/bootstrap/schedule_recovery_crash_test.go:124 +0x864
testing.tRunner(0x4bd687e69d48, 0x1095a3700)
    /usr/local/go/src/testing/testing.go:2193 +0xc4
created by testing.(*T).Run in goroutine 1
    /usr/local/go/src/testing/testing.go:2258 +0x3b8

goroutine 1 [chan receive]:
testing.(*T).Run(0x4bd687e69b08, {0x1065ce036?, 0x4bd6882ef8a8?}, 0x1095a3700)
    /usr/local/go/src/testing/testing.go:2266 +0x3cc
testing.runTests.func1(0x4bd687e69b08)
    /usr/local/go/src/testing/testing.go:2742 +0x38
testing.tRunner(0x4bd687e69b08, 0x4bd6882ef9d8)
    /usr/local/go/src/testing/testing.go:2193 +0xc4
testing.runTests({0x1065d1639, 0x20}, {0x10661671c, 0x33}, 0x4bd688018f18, {0x1096689d0, 0x94, 0x94}, {0x4bd6882efad8?, 0x1045ec438?, ...})
    /usr/local/go/src/testing/testing.go:2740 +0x400
testing.(*M).Run(0x4bd687f42780)
    /usr/local/go/src/testing/testing.go:2600 +0x578
github.com/Tangerg/flame/runtime/internal/testsupport.RunWithFirstPartyPackages(0x4bd687f42780)
    $WORKTREE/runtime/internal/testsupport/first_party.go:23 +0x3fc
github.com/Tangerg/flame/runtime/internal/bootstrap.TestMain(0x4bd687f42780)
    $WORKTREE/runtime/internal/bootstrap/first_party_packages_test.go:19 +0x154
main.main()
    _testmain.go:342 +0x88

goroutine 38 [select]:
database/sql.(*DB).connectionOpener(0x4bd687a77a00, {0x10959bec8, 0x4bd687fd3090})
    /usr/local/go/src/database/sql/sql.go:1266 +0x84
created by database/sql.OpenDB in goroutine 37
    /usr/local/go/src/database/sql/sql.go:846 +0x11c

goroutine 24 [runnable]:
database/sql.(*Tx).awaitDone(0x4bd687a6a090)
    /usr/local/go/src/database/sql/sql.go:2217 +0x30
created by database/sql.(*DB).beginDC in goroutine 37
    /usr/local/go/src/database/sql/sql.go:1930 +0x17c

goroutine 307 [select]:
github.com/Tangerg/go-sdk/mcp.(*ioConn).Read(0x4bd687afabe0, {0x10959d438, 0x4bd688302de0})
    $GOMODCACHE/github.com/!tangerg/go-sdk@v1.8.1-scope.1/mcp/transport.go:706 +0x10c
github.com/Tangerg/go-sdk/internal/jsonrpc2.(*Connection).readIncoming(0x4bd68848b2b0, {0x10959d438, 0x4bd688302de0}, {0x11832b400, 0x4bd687afabe0}, {0x10957f5b0, 0x4bd688302dd0})
    $GOMODCACHE/github.com/!tangerg/go-sdk@v1.8.1-scope.1/internal/jsonrpc2/conn.go:524 +0x54
created by github.com/Tangerg/go-sdk/internal/jsonrpc2.(*Connection).start.func1 in goroutine 192
    $GOMODCACHE/github.com/!tangerg/go-sdk@v1.8.1-scope.1/internal/jsonrpc2/conn.go:259 +0xdc

goroutine 305 [select]:
os/exec.(*Cmd).watchCtx(0x4bd688419a00, 0x4bd688392a80)
    /usr/local/go/src/os/exec/exec.go:808 +0x74
created by os/exec.(*Cmd).Start in goroutine 192
    /usr/local/go/src/os/exec/exec.go:794 +0x6ac

goroutine 306 [IO wait]:
internal/poll.runtime_pollWait(0x11832b200, 0x72)
    /usr/local/go/src/runtime/netpoll.go:351 +0xa0
internal/poll.(*pollDesc).wait(0x4bd687a81380?, 0x4bd687fcde80?, 0x1)
    /usr/local/go/src/internal/poll/fd_poll_runtime.go:84 +0x28
internal/poll.(*pollDesc).waitRead(...)
    /usr/local/go/src/internal/poll/fd_poll_runtime.go:89
internal/poll.(*FD).Read(0x4bd687a81380, {0x4bd687fcde80, 0x40, 0x40})
    /usr/local/go/src/internal/poll/fd_unix.go:170 +0x22c
os.(*File).read(...)
    /usr/local/go/src/os/file_posix.go:30
os.(*File).Read(0x4bd687e82898, {0x4bd687fcde80?, 0x4bd688092ce0?, 0x4bd688092e78?})
    /usr/local/go/src/os/file.go:144 +0x68
github.com/Tangerg/go-sdk/mcp.(*pipeRWC).Read(0x666564242f23223a?, {0x4bd687fcde80?, 0x5ffc0b80626?, 0x0?})
    $GOMODCACHE/github.com/!tangerg/go-sdk@v1.8.1-scope.1/mcp/cmd.go:59 +0x28
github.com/Tangerg/go-sdk/mcp.(*frameLimitReader).Read(0x4bd68840f5a0, {0x4bd687fcde80?, 0x0?, 0x0?})
    $GOMODCACHE/github.com/!tangerg/go-sdk@v1.8.1-scope.1/mcp/transport.go:604 +0x58
encoding/json/jsontext.(*decoderState).fetch(0x4bd68843a780)
    /usr/local/go/src/encoding/json/jsontext/decode.go:240 +0x350
encoding/json/jsontext.(*decoderState).consumeWhitespace(0x4bd68843a780, 0x0?)
    /usr/local/go/src/encoding/json/jsontext/decode.go:844 +0xa0
encoding/json/jsontext.(*decoderState).ReadValue(0x4bd68843a780, 0x4bd688092e70)
    /usr/local/go/src/encoding/json/jsontext/decode.go:702 +0x870
encoding/json/jsontext.(*Decoder).ReadValue(...)
    /usr/local/go/src/encoding/json/jsontext/decode.go:679
encoding/json.(*Decoder).Decode(0x4bd6881ccff0, {0x108b71bc8, 0x4bd687aa3710})
    /usr/local/go/src/encoding/json/v2_stream.go:78 +0x48
github.com/Tangerg/go-sdk/mcp.newIOConnLimited.func1()
    $GOMODCACHE/github.com/!tangerg/go-sdk@v1.8.1-scope.1/mcp/transport.go:550 +0x128
created by github.com/Tangerg/go-sdk/mcp.newIOConnLimited in goroutine 192
    $GOMODCACHE/github.com/!tangerg/go-sdk@v1.8.1-scope.1/mcp/transport.go:538 +0xc0

goroutine 192 [select]:
github.com/Tangerg/go-sdk/internal/jsonrpc2.(*AsyncCall).Await(0x4bd68840f4a0, {0x10959be90, 0x4bd6881cc4e0}, {0x108b831a8, 0x4bd687a815c0})
    $GOMODCACHE/github.com/!tangerg/go-sdk@v1.8.1-scope.1/internal/jsonrpc2/conn.go:435 +0x70
github.com/Tangerg/go-sdk/mcp.call({0x10959be90, 0x4bd6881cc4e0}, 0x4bd68848b2b0, {0x1065a1071, 0xf}, {0x10959b6e8?, 0x4bd687e828d0?}, {0x10959c8d8, 0x4bd687a815c0})
    $GOMODCACHE/github.com/!tangerg/go-sdk@v1.8.1-scope.1/mcp/transport.go:320 +0x9c
github.com/Tangerg/go-sdk/mcp.defaultSendingMethodHandler({0x10959be90, 0x4bd6881cc4e0}, {0x1065a1071, 0xf}, {0x10959cef8, 0x4bd688302e10})
    $GOMODCACHE/github.com/!tangerg/go-sdk@v1.8.1-scope.1/mcp/shared.go:164 +0x36c
github.com/Tangerg/go-sdk/mcp.clientMultiRoundTripMiddleware.func1.1({0x10959be90?, 0x4bd6881cc4e0?}, {0x1065a1071?, 0x10c834a00?}, {0x10959cef8?, 0x4bd688302e10?})
    $GOMODCACHE/github.com/!tangerg/go-sdk@v1.8.1-scope.1/mcp/mrtr.go:68 +0xf8
github.com/Tangerg/go-sdk/mcp.handleSend[...]({0x10959be90, 0x4bd6881cc4e0}, {0x1065a1071, 0xf}, {0x10959cef8, 0x4bd688302e10})
    $GOMODCACHE/github.com/!tangerg/go-sdk@v1.8.1-scope.1/mcp/shared.go:189 +0x74
github.com/Tangerg/go-sdk/mcp.(*Client).discover(0x4bd687ae46c0, {0x10959be90, 0x4bd6881cc4e0}, 0x4bd687ae4840)
    $GOMODCACHE/github.com/!tangerg/go-sdk@v1.8.1-scope.1/mcp/client.go:429 +0x2b4
github.com/Tangerg/go-sdk/mcp.(*Client).Connect(0x4bd687ae46c0, {0x10959bec8, 0x4bd687af6820}, {0x10957ca10?, 0x4bd688302d90?}, 0x0)
    $GOMODCACHE/github.com/!tangerg/go-sdk@v1.8.1-scope.1/mcp/client.go:328 +0x168
github.com/Tangerg/flame/runtime/internal/infra/integration/mcp.dial.func2({0x10959bec8, 0x4bd687af6820})
    $WORKTREE/runtime/internal/infra/integration/mcp/config.go:124 +0x220
github.com/Tangerg/flame/runtime/internal/infra/integration/mcp.connectSession({0x10959bec8, 0x4bd6880b2ff0}, {0x10959bec8, 0x4bd687fd3040}, 0x0, 0x4bd6883ecd78)
    $WORKTREE/runtime/internal/infra/integration/mcp/config.go:204 +0x398
github.com/Tangerg/flame/runtime/internal/infra/integration/mcp.dial({0x10959bec8, 0x4bd6880b2ff0}, {0x10959bec8, 0x4bd687fd3040}, 0x4bd687ae46c0, 0x5?)
    $WORKTREE/runtime/internal/infra/integration/mcp/config.go:133 +0x16c
github.com/Tangerg/flame/runtime/internal/infra/integration/mcp.(*Connections).dialAndSwap(0x4bd687fa61b0, {0x10959bec8, 0x4bd6880b2ff0}, 0x4bd68820bb90, 0x4bd68843a640, 0x0)
    $WORKTREE/runtime/internal/infra/integration/mcp/reconnect.go:105 +0x98
github.com/Tangerg/flame/runtime/internal/infra/integration/mcp.(*Connections).configure(0x4bd687fa61b0, 0x4bd68820bb90, 0x4bd68843a640)
    $WORKTREE/runtime/internal/infra/integration/mcp/reconnect.go:48 +0x86c
github.com/Tangerg/flame/runtime/internal/infra/integration/mcp.(*Attempt).connect(0x4bd68820bb90)
    $WORKTREE/runtime/internal/infra/integration/mcp/attempt.go:168 +0x398
github.com/Tangerg/flame/runtime/internal/infra/integration/mcp.(*Attempt).execute(0x4bd68820bb90)
    $WORKTREE/runtime/internal/infra/integration/mcp/attempt.go:134 +0xa0
github.com/Tangerg/flame/runtime/internal/infra/integration/mcp.(*Attempt).run(0x4bd68820bb90)
    $WORKTREE/runtime/internal/infra/integration/mcp/attempt.go:100 +0x8c
created by github.com/Tangerg/flame/runtime/internal/infra/integration/mcp.(*Connections).begin in goroutine 37
    $WORKTREE/runtime/internal/infra/integration/mcp/attempt.go:82 +0x664

goroutine 257 [chan receive]:
github.com/Tangerg/flame/runtime/internal/infra/integration/mcp.(*Attempt).Run(0x4bd68820bb90)
    $WORKTREE/runtime/internal/infra/integration/mcp/attempt.go:88 +0x30
github.com/Tangerg/flame/runtime/internal/application/integration/mcp.(*Coordinator).dispatchConnection.func1({0x10959bec8, 0x4bd6880b3040})
    $WORKTREE/runtime/internal/application/integration/mcp/connections.go:93 +0x13c
github.com/Tangerg/flame/runtime/internal/application/taskgroup.detach({0x10959bec8?, 0x4bd6880b3040?}, 0x0?)
    $WORKTREE/runtime/internal/application/taskgroup/taskgroup.go:78 +0x58
github.com/Tangerg/flame/runtime/internal/application/taskgroup.(*Group).StartLinked.func1()
    $WORKTREE/runtime/internal/application/taskgroup/taskgroup.go:94 +0x38
created by github.com/Tangerg/flame/runtime/internal/application/taskgroup.(*Group).StartLinked in goroutine 37
    $WORKTREE/runtime/internal/application/taskgroup/taskgroup.go:92 +0xa4
```

## Candidate test-source hashes

SHA-256 identifies the test source read by this gate; documentation is excluded.

```text
ed10ab38eca67a3020ea4486279a87cfe4fb25393dd9b83e10e2fad504734361  runtime/internal/bootstrap/cancel_terminal_restart_test.go
a9ca7d6236f1379990ef3f92bcefd16d9902b6e03c1e9705a35cb53ea31f87d0  runtime/internal/bootstrap/plugin_acceptance_approval_test.go
ee1882389bad30d4934ed1d487e26be0caec9f9d6e08be42ea756f443d7035c1  runtime/internal/bootstrap/plugin_acceptance_discovery_test.go
862e1b3d2b480f96bc559a18c8acdcc193005193851ed1905337abecfb80850a  runtime/internal/bootstrap/plugin_acceptance_mcp_test.go
9e16262f58ea86de7adf1166954e259b93dfba33201a93e6a75f2cd91e9f91e4  runtime/internal/bootstrap/plugin_lifecycle_test.go
bb69f7e4df7f89fe0f89d08beb6b1fdec093658bb58a3b43c9f7549c6fea96c4  runtime/internal/bootstrap/plugin_reviews_test.go
51453027551517b46bab32f74b3b0a411a46c4e2a52e95824c625c1f48baa9b5  runtime/internal/bootstrap/plugin_startup_stdio_test.go
60fc79e7efc50cb3bbb14c87250263332c25d764270ab30678286a34c13a0025  runtime/internal/bootstrap/plugin_startup_test.go
09300ee765aa1cec9f629543058c27bd62cba069df41b475c4d2c1e5dd59dcff  runtime/internal/bootstrap/protocol_lifecycle_test.go
e9ec84b03dddbf171dd292d555a8743945077f2d60dc0bdd422b59194006c696  runtime/internal/bootstrap/schedules_callback_loss_test.go
```

## Follow-up boundary

A temporary, isolated overlay records proxy frame metadata and backend startup
phases, with a 9s live CPU/stack snapshot. That bounded command exits 0 after
454.684s: execution/bootstrap pass 333.208/435.734s, with 385 top-level passes,
zero failures/skips and all eight cuts passing (parent 52.33s). It produces 73
process logs and 39 complete proxy discovery traces, with no reproduced discovery
deadline. Its only 9s stack belongs to a backend already inside `server.Run`
during the executable CLI case. The watcher finds no owned proxy still awaiting
its initial response, so no native child sample is taken. Killed, unclosed CPU
profiles are not qualified measurements.

These diagnostic bytes/timing and normal-path observations do not explain the
failed gate or qualify unmodified-source acceptance. The initial handshake's
missing response still lacks a cause. The historical safe-steer deadline remains
independently unresolved. Do not infer CPU contention, a lease failure, schema
cost or a proxy defect from these passing observations. Only a captured causal
failure can support such a conclusion. Temporary overlays/profiles are removed
after inspection; the representative actual failure above remains retained.
