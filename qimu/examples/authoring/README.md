# Public author exercises

These paired exercises implement new document plugins with Qimu's public APIs.
They are maintained examples and contract tests, not results from independent
third-party authors. Both reuse the document consumer's public contract owner.
Neither calls the document workbench's built-in plugin factories.

| Task                              | Go author                                    | TypeScript author                                     |
| --------------------------------- | -------------------------------------------- | ----------------------------------------------------- |
| FileReader and imported Converter | Actual files and exact source revision       | Actual HTTP source and original AbortSignal           |
| Validator contribution            | Two validators with explicit ordering        | Title validator with public metadata                  |
| Multiple Points                   | One Converter/Validator snapshot and Capture | One snapshot and acquisition for both Points          |
| Resource-valued result            | HTML file View protected by the original Use | HTML Blob URL Acquired handle under the original Use  |
| Cancellation                      | Cancel and join an admitted reader           | Abort and join an active HTTP source body             |
| Partial setup failure             | Missing template after workspace acquisition | Staged Validator and Blob URL followed by setup error |

The Go [native exercise](native/README.md) builds without a database or transport.
The [TypeScript exercise](browser/README.md) uses the real Go Host, SQLite and
authenticated control Bridge through `cmd/authorhost`. Native function values
stay in their original language and execution Realm; contract identity does not
make a native function cross-language callable. The document workbench separately
provides the actual browser-engine/iframe exercise.

To verify from this module, run `GOWORK=off go test -race
./examples/authoring/native`, then follow the TypeScript README. Repository
verification builds the trusted Host fixture and runs the TypeScript workflow as
a required check. It does not skip the exercise when a binary is missing.

For release acceptance, create independent Go and npm projects from these author
sources and use only the exact public Go module and packed npm artifacts. Their
own module files and npm lock must not use workspace replacements or checkout
links. Regenerate contracts from the matching released fixture if needed. Record
the concrete release, commands, materials consulted, errors and corrections,
and measured command durations outside the source tree. Complete author learning
time was not measured during implementation; do not turn a command duration into
an independent author's completion time.
