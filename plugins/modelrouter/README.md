# Model router

This backend owns one private routing policy in `modelrouter.db`. Its typed policy
operations and model-selection contribution share that owner. It has no UI, MCP
tools, provider clients, credentials or host callbacks.

Install and enable the built package, then explicitly select `modelrouter-selection`
for `flame.model-selection.v2`. Enabling a package alone does not choose it as the
model selector. The initial empty policy preserves each caller's exact proposal.
Inspect `flame.model-router/get-policy.v1`, then replace at that revision through
`flame.model-router/replace-policy.v1` with a command identity. An example input is:

```json
{
  "expectedRevision": 1,
  "rules": [
    {
      "use": "utility",
      "selection": {
        "provider": "openai",
        "model": "gpt-5-mini",
        "reasoningEffort": ""
      },
      "ceiling": {
        "inputKind": "text",
        "maxInputBytes": 16384,
        "maxOutputTokens": 1024
      }
    }
  ]
}
```

Each use has at most one rule. A matching rule returns the complete provider/model/
reasoning identity. A ceiling additionally requires both captured bounds to fit;
otherwise the original proposal is preserved. An omitted ceiling matches that use
unconditionally. Rules do not rank competing routes, guess task difficulty from
conversation content, discover models, or retry failed selections.

The host publishes three actual uses: `interaction`, `utility` and `independent`.
Utility budgets bound model-visible text bytes. Independent budgets bound encoded
request bytes, so their ceiling uses `inputKind: "request"`. Interaction has no
captured caller byte budget and accepts only an unconditional rule. Missing or
wrongly typed envelopes are errors, not evidence that a task is small. A bounded
utility rule provides an explicit way to send small background tasks to a cheaper
configured model without pretending to estimate monetary cost.

Policy replacement, its revision and its original-input command receipt commit in
one private transaction. Repeated command identities return the original result;
changed inputs conflict. Stale revisions refuse replacement, including across
backend restart. Missing or corrupt stored policy refuses startup rather than
recreating a default policy. Generated management contracts live beside the
generated selection contract; regenerate with:

```sh
go generate ./runtime/internal/plugins/modelrouter/rpc
```

Runtime admits the selected identity against its provider configuration and actual
catalog capacity, constructs the model, enforces the original caller envelope and
prices the actual endpoint. An unavailable selected model fails the use case.
Accepted Runs freeze their selection; retries do not reroute. Session preferences,
waiting executions, compaction and accounting remain core facts. Policy changes
affect only future selection queries.
