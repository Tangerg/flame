# Feature operations

Generated from the feature's Go-owned method contracts.

| Method | Kind | Target | Description |
| --- | --- | --- | --- |
| `flame.skill-learning/archive.v2` | command | installation | Return an optional absolute activity cutoff for the supplied observation time. The caller alone admits and commits archival. |
| `flame.skill-learning/mine.v2` | command | installation | Mine a completed Run using bounded collaborators. Commit an admitted proposal and its original mining receipt in the same transaction. |
| `flame.skills/approve.v1` | command | installation | Record a human decision for the current proposal and retain its original publication permit. |
| `flame.skills/capture.v1` | command | installation | Record an interaction's original Skill source choices for exact receipt recovery. |
| `flame.skills/confirm.v1` | command | installation | Close the original publication after the authorized filesystem owner confirms publication or a definite precondition conflict. |
| `flame.skills/discover.v1` | query | installation | Select among the host-admitted Skill readers for a human catalog. |
| `flame.skills/propose.v1` | command | installation | Stage a reviewed library draft under its immutable originating command. |
| `flame.skills/reject.v1` | command | installation | Withdraw an exact pending proposal before approval. |
| `flame.skills/reviews.v1` | query | installation | List outstanding proposal commands in the admitted libraries, including publications awaiting confirmation. |

Capability `flame.skill-learning.v2` exposes the declared methods. Required backend protocol: `io.github.tangerg.frame/portable/1`.

`capability.generated.json` preserves the canonical callable contract; `schema.json` and the method references are presentation projections.
