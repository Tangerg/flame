import { mkdir, readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { describe, expect, it } from "vitest";
import { RpcError, asSessionId } from "@flame/runtime-client";
import { collectRunEvents, nextRuntimeEvent, originalOpeningEvents } from "./observation";
import { managedSkillName, useRuntimeFixture } from "./fixture";

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: skills", () => {
  const fixture = useRuntimeFixture("skills");

  it("reconciles skill archive and restore through skills.changed", async () => {
    const workspace = fixture.client.workspace({ path: fixture.root });
    await expect(fixture.client.skills.listLibrary()).resolves.toMatchObject({
      data: [expect.objectContaining({ name: managedSkillName, lifecycle: "active" })],
    });
    const discoveredSkills = (await workspace.skills.listDiscovered()).skills;
    expect(discoveredSkills).toContainEqual(
      expect.objectContaining({ name: managedSkillName, scope: "user" }),
    );
    const retainedSkills = discoveredSkills.filter(
      (skill) => !(skill.scope === "user" && skill.name === managedSkillName),
    );

    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      {
        topics: ["skills.changed", "files.changed"],
        watches: [{ watchId: "skills-authored", workspace: { path: fixture.root } }],
      },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    await fixture.client.skills.archive(managedSkillName);
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });
    await expect(fixture.client.skills.listLibrary()).resolves.toMatchObject({
      data: [expect.objectContaining({ name: managedSkillName, lifecycle: "archived" })],
    });
    await expect(workspace.skills.listDiscovered()).resolves.toMatchObject({
      skills: retainedSkills,
    });

    await fixture.client.skills.restore(managedSkillName);
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });
    await expect(fixture.client.skills.listLibrary()).resolves.toMatchObject({
      data: [expect.objectContaining({ name: managedSkillName, lifecycle: "active" })],
    });
    await expect(workspace.skills.listDiscovered()).resolves.toMatchObject({
      skills: discoveredSkills,
    });

    const projectSkillName = "external-project-skill";
    const projectSkillDirectory = join(fixture.root, ".flame", "skills", projectSkillName);
    await mkdir(projectSkillDirectory, { recursive: true });
    await writeFile(
      join(projectSkillDirectory, "SKILL.md"),
      `---\nname: ${projectSkillName}\ndescription: Observe an externally authored project Skill.\n---\n\nUse the project workflow.\n`,
    );
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });
    await expect(workspace.skills.listDiscovered()).resolves.toMatchObject({
      skills: expect.arrayContaining([
        expect.objectContaining({ name: projectSkillName, scope: "repository" }),
      ]),
    });

    const userSkillName = "external-user-skill";
    const userSkillDirectory = join(fixture.runtimeStore, "skills", userSkillName);
    await mkdir(userSkillDirectory, { recursive: true });
    await writeFile(
      join(userSkillDirectory, "SKILL.md"),
      `---\nname: ${userSkillName}\ndescription: Observe an externally authored user Skill.\n---\n\nUse the user workflow.\n`,
    );
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });
    await expect(fixture.client.skills.listLibrary()).resolves.toMatchObject({
      data: expect.arrayContaining([
        expect.objectContaining({ name: userSkillName, lifecycle: "active" }),
      ]),
    });

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);

  it("keeps project skill precedence when the personal library changes", async () => {
    const workspaceRoot = join(fixture.root, "skill-precedence");
    const skillDirectory = join(workspaceRoot, ".flame", "skills", managedSkillName);
    await mkdir(skillDirectory, { recursive: true });
    await writeFile(
      join(skillDirectory, "SKILL.md"),
      `---\nname: ${managedSkillName}\ndescription: Project-owned review.\n---\n\nFollow the project review process.\n`,
    );
    const workspace = fixture.client.workspace({ path: workspaceRoot });
    const projectSkill = {
      name: managedSkillName,
      description: "Project-owned review.",
      scope: "repository",
    };
    try {
      await fixture.client.skills.archive(managedSkillName);
      expect((await workspace.skills.listDiscovered()).skills).toContainEqual(projectSkill);
      await fixture.client.skills.restore(managedSkillName);
      expect((await workspace.skills.listDiscovered()).skills).toContainEqual(projectSkill);
    } finally {
      const managed = (await fixture.client.skills.listLibrary()).data.find(
        (skill) => skill.name === managedSkillName,
      );
      if (managed?.lifecycle === "archived") {
        await fixture.client.skills.restore(managedSkillName);
      }
    }
  }, 30_000);

  it("reviews project and user skill proposals produced by a real run tool", async () => {
    const workspaceRoot = join(fixture.root, "workspace-skill-proposals");
    await mkdir(workspaceRoot);
    const workspace = fixture.client.workspace({ path: workspaceRoot });
    const streamController = new AbortController();
    const subscription = await fixture.client.runtimeEvents.subscribe(
      { topics: ["skills.changed"] },
      streamController.signal,
    );
    const runtimeEvents = subscription.events[Symbol.asyncIterator]();

    const projectSession = await fixture.client.sessions.create({
      workspace: { path: workspaceRoot },
      title: "HTTP project Skill proposal",
    });
    const projectRun = await fixture.client.runs.start({
      sessionId: asSessionId(projectSession.id),
      input: [{ type: "text", text: "E2E_SKILL_PROPOSAL_PROJECT save this workflow." }],
    });
    const projectEvents = await collectRunEvents(originalOpeningEvents(projectRun));
    expect(projectEvents).toEqual(
      expect.arrayContaining([
        expect.objectContaining({
          event: expect.objectContaining({
            type: "item.completed",
            item: expect.objectContaining({
              type: "toolCall",
              tool: expect.objectContaining({ name: "propose_skill" }),
            }),
          }),
        }),
      ]),
    );
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });

    const projectProposals = await workspace.skills.listProposals();
    expect(projectProposals.data).toEqual([
      expect.objectContaining({
        name: "e2e-project-proposal",
        scope: "repository",
        origin: "requested",
        sourceSession: projectSession.id,
        description: "Preserve the project-side HTTP E2E workflow.",
        instructions: "Run the project-side HTTP E2E workflow and report its result.",
        revision: expect.stringMatching(/^[a-f0-9]+$/),
      }),
    ]);
    const projectProposal = projectProposals.data[0];
    if (!projectProposal) throw new Error("project proposal disappeared before review");
    const projectRef = {
      name: projectProposal.name,
      revision: projectProposal.revision,
      scope: projectProposal.scope,
    };
    await workspace.skills.approveProposal(projectRef);
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });
    await expect(workspace.skills.listProposals()).resolves.toMatchObject({ data: [] });
    await expect(workspace.skills.listDiscovered()).resolves.toMatchObject({
      skills: expect.arrayContaining([
        expect.objectContaining({ name: projectProposal.name, scope: "repository" }),
      ]),
    });
    await expect(
      readFile(join(workspaceRoot, ".flame", "skills", projectProposal.name, "SKILL.md"), "utf8"),
    ).resolves.toContain(projectProposal.instructions);
    await expect(workspace.skills.approveProposal(projectRef)).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && error.data.type === "revision_conflict",
    );

    const userSession = await fixture.client.sessions.create({
      workspace: { path: workspaceRoot },
      title: "HTTP user Skill proposal",
    });
    const userRun = await fixture.client.runs.start({
      sessionId: asSessionId(userSession.id),
      input: [{ type: "text", text: "E2E_SKILL_PROPOSAL_USER save this workflow." }],
    });
    await collectRunEvents(originalOpeningEvents(userRun));
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });
    const userProposal = (await workspace.skills.listProposals()).data.find(
      (proposal) => proposal.name === "e2e-user-proposal",
    );
    if (!userProposal) throw new Error("user proposal disappeared before review");
    expect(userProposal).toMatchObject({
      scope: "user",
      origin: "requested",
      sourceSession: userSession.id,
    });
    await workspace.skills.rejectProposal({
      name: userProposal.name,
      revision: userProposal.revision,
      scope: userProposal.scope,
    });
    await expect(nextRuntimeEvent(runtimeEvents, "skills.changed")).resolves.toMatchObject({
      type: "skills.changed",
    });
    await expect(workspace.skills.listProposals()).resolves.toMatchObject({ data: [] });
    expect(
      (await fixture.client.skills.listLibrary()).data.some(
        (skill) => skill.name === userProposal.name,
      ),
    ).toBe(false);

    streamController.abort();
    await runtimeEvents.return?.();
  }, 30_000);
});
