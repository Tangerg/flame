# Reviews analyzer example

This is a separate ordinary plugin package. It contributes a deterministic
implementation to the point owned by Reviews, through public callable and value
contracts. It has no publisher privilege, database connection or Runtime internal
import. Reviews reads and admits its findings; the analyzer cannot change a review.

The executable consumes the public types and contracts in the independent Reviews
author module at `backend/contract`. Snapshot admission and finding validation
are public pure contracts; private stores and review transitions are inaccessible. Its `unresolvedReview` finding refers to the inspected review ID
and revision. A resolved snapshot has no findings. Invalid snapshots are refused.

From the repository root, prepare current dependencies and contracts, then
assemble a new release outside the source tree:

```sh
make setup
make generate
mkdir -p out/plugins
mkdir out/plugins/review-analyzer
cp -R examples/plugins/reviews/analyzer/package/. out/plugins/review-analyzer/
mkdir -p out/plugins/review-analyzer/bin
go build -trimpath -ldflags='-s -w' -o out/plugins/review-analyzer/bin/review-analyzer ./examples/plugins/reviews/backend/cmd/review-analyzer
```

Install the absolute `out/plugins/review-analyzer` directory through
`plugins install`, then enable it through `plugins set-enablement`. Enable it
before activating Reviews; Reviews' optional named analyzer collection is
captured at setup. Deliberate changes require reloading that consumer entry.

`plugin.json` owns package identity, `mcp.json` owns process launch, and the
referenced namespace declaration binds `analyzer` plus the point value
`unresolved-review`. Contract files and the TypeScript method projection are
generated from the Reviews consumer's Go types. Do not edit a generated schema
or add an analyzer branch to the generic package loader.

`TestInstalledReviewsPointUsesCapturedCollaboratorsAndCurrentConsumerLifetime` in
Runtime bootstrap builds both installed executables and exercises their actual
Core callback route. It verifies original review identity/revision, optional absence
on disable, reenable, frozen setup authority and explicit collection refresh.
The root check validates the Reviews module with `GOWORK=off` against the frozen
module artifacts prepared by `make setup`. Published consumers must select a
release containing the public SDK; the candidate file proxy is not publication.
Native component qualification belongs to `make check-native`.
