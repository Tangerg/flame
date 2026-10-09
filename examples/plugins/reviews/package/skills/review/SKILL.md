---
name: review
description: Inspect and explicitly update the review example backend.
---

Discover the installation's `list_reviews` and `update_review` tools. Read the
current review before proposing an update. Submit its exact ID and inspected
revision with the user's chosen status (`open` or `resolved`). A revision
conflict requires another read and a new user decision; do not silently rebase.

Runtime supplies invocation identity outside the tool arguments. Do not invent
an argument or use a provider ToolCall ID as a retry key. Uncertain execution
requires investigation; do not submit a replacement mutation automatically.
