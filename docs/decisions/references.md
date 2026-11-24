# References

A reference pins something the user pointed at, such as lines of a file, a quoted
reply or a slide element, so the Agent works on exactly that and not on whatever a
view shows later.

## Owners

The producer of a reference owns what it points at. A host reader or an installed
component writes the reference when the user adds it: a one-line `label` people
see and the `text` the model receives, naming the exact object, version and
position. A slide reference names the saved revision; a file reference carries
the lines the user saw.

The Composer owns an unsent reference. The user can open it to read exactly what
the model will receive and remove it; nothing edits it, because only its producer
knows what it means. Adding the same reference twice keeps one. Drafts persist
references with their text, and editing or resending a message brings its
references back.

Runtime owns a sent reference. It is a `reference` content block of the user
message, stored and exported as sent, refused anywhere but user input, and
rendered for the model between a frame that names its label, so the model can
tell what the user pointed at from what the user wrote.

## Not yet

A reference carries no structured target. Opening its object again, or reporting
that the object changed or was deleted, needs a domain to resolve the target; that
arrives with its first consumer, the pull-request review, which also supplies
commit-pinned code lines. Local diff lines are not quotable until then.
