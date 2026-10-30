# Model catalog admission

`NewAdmission` captures an exact selection and the model facts supplied by its
owner. It validates the model-owned reasoning vocabulary and snapshots the facts.
`Admission.AdmitInput` applies that same capture's accepted modalities. This
package performs no provider I/O and never supplies another provider or model.

A catalog miss remains admissible because compatible endpoints may expose private
models. The configured Runtime/provider remains their execution authority; catalog
absence does not establish model availability or invent token limits. The selected
adapter owns published facts; this package never reads a process-wide catalog.

The former zero-value `Selections` API is removed. Callers must supply the
selected adapter's captured metadata, or explicit absence when it is unknown.
