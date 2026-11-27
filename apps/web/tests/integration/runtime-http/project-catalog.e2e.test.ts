import { describe, expect, it } from "vitest";
import { HTTP_HEADERS } from "@flame/runtime-client/wire";
import { readRuntimeProjectSummaries } from "@/features/projects/adapters/runtimeProjects";
import { useRuntimeFixture } from "./fixture";
import { within } from "./observation";

describe("Go Runtime ↔ HTTP ↔ Web Project catalog", () => {
  const fixture = useRuntimeFixture("project-catalog");

  it("observes a Project once when it moves between active and archived", async () => {
    const project = await fixture.client.projects.create({ name: "Archive during a catalog read" });
    const activeRead = Promise.withResolvers<void>();
    const releaseArchived = Promise.withResolvers<void>();
    const scopes: Array<boolean | undefined> = [];
    const client = fixture.createRuntimeClient(async (url, init) => {
      const method = new Headers(init?.headers).get(HTTP_HEADERS.method);
      if (method !== "projects.list") return fetch(url, init);
      const request = JSON.parse(String(init?.body)) as { params: { archived?: boolean } };
      const archived = request.params.archived;
      scopes.push(archived);
      if (archived === true) await releaseArchived.promise;
      const response = await fetch(url, init);
      if (archived !== true) activeRead.resolve();
      return response;
    });
    const reading = readRuntimeProjectSummaries(client);
    try {
      await within(activeRead.promise, "the first valid catalog response");
      await fixture.client.projects.update({
        projectId: project.id,
        expectedRevision: project.revision,
        archived: true,
      });
      releaseArchived.resolve();

      const visible = (await reading).filter(({ id }) => id === project.id);
      expect(visible).toHaveLength(1);
      expect(scopes).toEqual([undefined]);
      expect(
        (await readRuntimeProjectSummaries(fixture.client)).find(({ id }) => id === project.id),
      ).toMatchObject({ archived: true });
    } finally {
      releaseArchived.resolve();
      await reading.catch(() => {});
      await client.close();
    }
  }, 30_000);

  it("keeps continuation cursors in the chosen archive scope", async () => {
    const active = await fixture.client.projects.create({ name: "Active Project catalog entry" });
    const archived = await fixture.client.projects.create({
      name: "Archived Project catalog entry",
    });
    await fixture.client.projects.update({
      projectId: archived.id,
      expectedRevision: archived.revision,
      archived: true,
    });

    const { nextCursor } = await fixture.client.projects.list({ limit: 1 });
    if (!nextCursor) throw new Error("the full catalog should span two pages");
    await expect(
      fixture.client.projects.list({ limit: 1, cursor: nextCursor, archived: false }),
    ).rejects.toMatchObject({ data: { type: "invalid_params" } });
    await expect(
      fixture.client.projects.list({ limit: 1, cursor: nextCursor, archived: true }),
    ).rejects.toMatchObject({ data: { type: "invalid_params" } });

    const ids = async (scope?: { archived: boolean }) =>
      (await fixture.client.projects.list({ limit: 1, ...scope }).autoPagingToArray()).map(
        ({ id }) => id,
      );
    const all = await ids();
    expect(all.filter((id) => id === active.id || id === archived.id)).toEqual([
      archived.id,
      active.id,
    ]);
    expect(all).toHaveLength(new Set(all).size);
    const activeIds = await ids({ archived: false });
    expect(activeIds).toContain(active.id);
    expect(activeIds).not.toContain(archived.id);
    const archivedIds = await ids({ archived: true });
    expect(archivedIds).toContain(archived.id);
    expect(archivedIds).not.toContain(active.id);
  }, 30_000);
});
