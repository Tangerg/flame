# Guardrails

This backend owns an exact protected-phrase policy in private `guardrails.db`.
Its `flame.model-input.v1` decoration inspects the complete composed Scope request
before provider dispatch. A matching JSON string token returns `protected_content`;
an allowed request is returned unchanged. The backend has no UI, model Tools,
provider client or host callbacks. It does not retain inspected requests.

Install and enable the built package and include `guardrails-input` in the input
contract's composition order. Inspect `flame.guardrails/get-policy.v1`, then change
the policy with `flame.guardrails/replace-policy.v1` and a command identity:

```json
{
  "expectedRevision": 1,
  "phrases": ["confidential@example.test", "internal-project-codename"]
}
```

The initial empty list allows requests. Each phrase is a case-sensitive UTF-8
substring, up to 512 bytes; at most 32 distinct, nonblank phrases are admitted.
Replacing with `[]` explicitly clears the policy. Inspection returns only revision
and phrase count. Refusals contain a stable category and no matching value. Policy
values enter through ordinary typed mutations and follow Runtime's original-input
command retention as well as this backend's private policy and receipt storage.

JSON string decoding covers message/context text, Tool definitions and schemas,
request options and metadata, including escaped strings and object member names.
The matcher does not classify arbitrary PII, decode binary media, normalize case,
or interpret application-encoded strings or numeric values as text. Its guarantee
is exact protected phrases in the decoded Scope JSON strings. Rejection preserves
the captured model, Tool contracts, output envelope and authored conversation.

Policy replacement and its original-input receipt commit atomically at the
inspected revision. Reused identities replay the original summary; changed inputs
conflict. Missing policy rows or phrase lists, corrupt snapshots and unavailable
contributors refuse rather than silently supplying an empty policy. Each new
input query uses the current policy; prior provider exchanges and authored history
retain their existing core owners. Runtime owns ordering, final request admission,
execution failure and accounting.

Regenerate the management and input contracts with:

```sh
go generate ./runtime/internal/plugins/guardrails/rpc
```
