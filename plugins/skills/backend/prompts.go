package main

const skillMinerPrompt = `You are mining an Agent conversation for a REUSABLE skill worth saving for future sessions.

A skill is a class-level, reusable procedure — "how to do X in this kind of project" — not a narration of this one task. Only propose a skill when the conversation demonstrates a non-obvious, repeatable procedure a future agent would benefit from. Prefer a general, umbrella skill covering a class of tasks over a narrow one-off, and write it to apply the next time a similar task arises.

Do NOT propose a skill for any of these:
- environment or dependency failures and their workarounds (transient, machine-specific)
- negative assertions about tools ("tool X doesn't work", "flag Y is unsupported")
- errors that were already resolved during this conversation (transient)
- a one-off task narrative with no reusable procedure
- anything already obvious from reading the project's source or its standard docs

If instead the conversation shows that a skill the agent LOADED this session was wrong, outdated, or corrected by the user, respond with a single line and nothing else:
REVISE: <the exact name of that loaded skill>

If nothing in the conversation deserves a saved or revised skill, respond with exactly NO_SKILL.

Otherwise output a complete SKILL.md for a NEW skill and nothing else — no preamble, no code fence:
---
name: <lowercase-hyphenated-identifier>
description: <what the skill does and WHEN to use it, one or two sentences>
---
<the skill body: concise, imperative instructions the agent will follow next time>`

const skillRevisePrompt = `You are revising an existing skill because the conversation shows it was wrong, outdated, or corrected by the user.

You are given the skill's CURRENT SKILL.md and the CONVERSATION. Produce the COMPLETE corrected SKILL.md: keep the same name, preserve everything still correct, and change only what the conversation shows is wrong. Do not invent changes the conversation does not justify.

If the current skill needs no change, respond with exactly NO_SKILL.

Otherwise output only the corrected SKILL.md — no preamble, no code fence:
---
name: <the same lowercase-hyphenated name>
description: <what it does and WHEN to use it>
---
<the corrected body>`
