# Project ownership

A Project is the user's logical unit of continuing work. It does not depend on a
directory: a Project can exist before any repository is chosen, and two Projects
can share one repository while keeping separate instructions and memory.

## Owners

Runtime owns the Project aggregate: stable identity, name, instructions, archive
state and revision. A plugin cannot own it. Session membership is checked when a
Session changes and is read whenever a Run opens a Segment, and both are Runtime
transitions; an optional process would otherwise sit on every Run admission.

The Session aggregate owns its membership in at most one Project. Membership
changes only through `sessions.update` against the observed Session revision while
the Session is idle, the same rule that guards a workspace change. A Run therefore
never observes its Project change underneath it. Joining requires an existing
Project; leaving needs no Project state.

Each domain owns the membership of its own objects. A plugin that records
project-scoped objects stores the captured Project identity with them and answers
project-filtered reads. There is no central link table that could disagree with an
object's owner, and the project home is a projection assembled from those reads.

Workspace keeps its meaning: the filesystem root that file Tools, hooks, agent
documents and approval rules resolve. A Project neither selects nor implies one.
`projectRoot`, project-level hooks and the `project` approval-rule scope still name
the repository directory above a workspace, not this aggregate. Rename them when a
rule or hook first needs to target a Project.

## Run scope

When a Segment opens, Runtime reads the Session's Project together with its
workspace and captures both in the execution scope. Project instructions enter the
system message composed at Run start, which the executor checkpoint retains;
editing them affects later Runs only. Their provenance names the Project revision,
so a trajectory export shows which instructions each model request carried.
Every installed Tool receives the captured Project in `plugin.ToolContext`, and
model-context contributions receive it in their request; neither accepts one
from model arguments.

## Plugin objects

The Knowledge base and Deliverables store the captured Project with the objects
they create. A Knowledge base document with a Project is retrieved only inside
that Project, while shared documents are retrieved everywhere; another Project's
document reads as absent. A deliverable records the Project it was made in, and
inside a Project the archive lists only that Project's documents.

## Memory

Memory's project target is the captured Project when the Session has one, and the
workspace directory otherwise. Directory memories keep their original scope: a
Session in a Project never reads them implicitly, so two Projects in one
repository cannot leak a directory's private facts into each other.

## Lifecycle

Archiving moves a Project out of the default list. It changes no Session, Run,
memory or plugin object, and restoring returns the same membership. Nothing
cascades from a Project; removing a Session from a Project clears only that
Session's membership.

## Data

The Project table and Session membership change the Runtime schema. Under the
pre-release data policy an existing data directory is refused; open a fresh one.
Session export carries no Project, so an imported Session starts unassigned in its
new Runtime. Memory keeps its storage: directory targets stay valid and Project
targets use the Project identity.
