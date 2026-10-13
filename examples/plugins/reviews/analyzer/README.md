# Reviews analyzer example

This is a separate ordinary plugin package. It contributes a deterministic
implementation to the point owned by Reviews, through public callable and value
contracts. It has no publisher privilege, database connection or Runtime internal
import. Reviews reads and admits its findings; the analyzer cannot change a review.

The executable consumes the public types and contracts in the independent Reviews
author module. Its `unresolvedReview` finding refers to the inspected review ID
and revision. A resolved snapshot has no findings. Invalid snapshots are refused.

From `examples/plugins/reviews/backend` in the repository workspace:

```sh
go generate ./...
mkdir -p ../analyzer/package/bin
go build -trimpath -ldflags='-s -w' -o ../analyzer/package/bin/review-analyzer ./cmd/review-analyzer
```

Install the absolute `analyzer/package` directory through `plugins install`, then
enable its installation through `plugins set-enablement`. Enable it before
activating Reviews; Reviews' optional named analyzer collection is captured at
setup. Deliberate later changes require reloading that consumer entry.

`plugin.json` owns package identity, `mcp.json` owns process launch, and the
referenced namespace declaration binds `analyzer` plus the point value
`unresolved-review`. Contract files and the TypeScript method projection are
generated from the Reviews consumer's Go types. Do not edit a generated schema
or add an analyzer branch to the generic package loader.

The final acceptance phase must exercise both installed executables and their
Core callback route. The current repository workspace build does not claim the
new SDK is available at the older published Runtime version in `go.mod`.
