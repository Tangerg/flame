# Native document author task

This is a maintainer-authored public-API example, not an external-user study. The
author writes a new implementation and consumer while reusing
`document.NewContracts`, `DocumentRef`, `FileReader`, `Converter` and `Validator`.
It does not call the document workbench's built-in plugin factories, import Qimu
internals, define a second contract identity, or hand-copy descriptor hashes.

`plugins.go` defines three native Units. The FileReader owns an actual `os.Root`,
checks the requested content revision and limits source size. The Converter
imports that exact FileReader at setup, materializes an admitted template in an
owned workspace, and performs its own uppercase-and-HTML-escape conversion. Two
authored validators contribute to the shared validators Point. The release digest
comes from the embedded authored implementation bytes.

`workflow.go` is the application consumer. It takes one snapshot of Converters and
Validators, applies explicit priority and ordering policies, and captures both
selections together. `Prepared.Acquire` enters that original plan once. A changed
validator set refuses the entire plan before conversion.

The acquired result is a `View` backed by an actual temporary HTML file. Its file
and selected providers belong to the original Qimu Use, which in turn belongs to
the supplied caller Lifetime. `WithReader` enters that same lifetime for every
access; the reader must not escape its callback. Cancellation seals new reads,
cancels an admitted reader, then waits for that reader before closing and removing
the file. `Close` joins the original Use. The Converter's public return contract
remains plain data; the consumer explicitly constructs this resource-valued View.

Configuration names source and working directories plus an exact template
DocumentRef. A missing template deliberately exercises a genuine partial setup:
the workspace was already acquired and registered, the original cleanup removes
it, no Converter becomes public, and correcting configuration permits a fresh
activation. No test-only callback is injected into a framework owner.

## Run

From the Qimu module:

```sh
GOWORK=off go test -race -v ./examples/authoring/native
```

The tests use temporary directories and only public Qimu APIs. They cover the
authored FileReader import, conversion and validators, multi-Point staleness,
later use of a resource result, provider protection until original release,
controlled cancellation/reader joining, partial-setup repair, and a changed
document revision.

For an independent author run, copy this directory's `.go` files into a fresh Go
module, require the exact released Qimu version, and run `GOWORK=off go test -race
-v .` with readonly module files after dependency resolution. The tests are in
the local consumer package so they execute the copied author implementation,
not an imported prebuilt author example. The final acceptance record must name
the actual release artifact and the command result; a workspace test is only
source verification.

## Materials and ownership choices

The maintainer consulted Qimu's root README, its public `Setup`, `Import`,
`Snapshot`, `SelectOne`, `SelectMany`, `Capture`, `Acquire`, `Lifetime` and `Work`
APIs, and the document example's public contract declarations. Prior knowledge
from maintaining the implementation is part of this trial and is not hidden.

Unit declarations repeat a public export name at `Provide`/`Contribute` so the
Host can check actual exports against the declaration. Import slot names recur at
the typed `Import` call for the same reason. Contract names/digests and provider
selection are not repeated in the plugin's method body. Source revision checking,
validator policy and the View's document report remain application facts.

Run duration, first successful command time and any observed errors belong in the
external author acceptance record. Test execution time is not a measurement of
the author's learning or implementation time.
