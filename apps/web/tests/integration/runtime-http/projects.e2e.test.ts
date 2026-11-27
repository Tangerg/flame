import { describe, expect, it } from "vitest";
import { QueryObserver } from "@tanstack/react-query";
import { HTTP_HEADERS } from "@flame/runtime-client/wire";
import {
  createRuntimeProjectCommands,
  readRuntimeProjectSummaries,
} from "@/features/projects/adapters/runtimeProjects";
import { PROJECTS_KEY, type ProjectSummary } from "@/features/projects/application/projectQueries";
import { queryClient } from "@/lib/queryClient";
import { useRuntimeFixture } from "./fixture";
import { within } from "./observation";

describe("Go Runtime ↔ HTTP ↔ Web Projects", () => {
  const fixture = useRuntimeFixture("projects");

  it("keeps a newer observed Project when an earlier edit acknowledgement arrives", async () => {
    const created = await fixture.client.projects.create({
      name: "Project awaiting its edit reply",
    });
    const committed = Promise.withResolvers<void>();
    const release = Promise.withResolvers<void>();
    const editing = fixture.createRuntimeClient(async (url, init) => {
      const response = await fetch(url, init);
      if (new Headers(init?.headers).get(HTTP_HEADERS.method) === "projects.update") {
        committed.resolve();
        await release.promise;
      }
      return response;
    });
    const queryKey = [PROJECTS_KEY];
    const read = () =>
      queryClient.fetchQuery({
        queryKey,
        queryFn: ({ signal }) => readRuntimeProjectSummaries(fixture.client, signal),
      });
    const commands = createRuntimeProjectCommands(() => editing);
    let edit: ReturnType<typeof commands.update> | undefined;
    try {
      const original = (await read()).find((project) => project.id === created.id);
      if (!original) throw new Error("the Project was not listed");
      edit = commands.update(original, { name: "Earlier Project update" });

      await within(committed.promise, "committing the first Project edit");
      const saved = await fixture.client.projects.get(created.id);
      const latest = await fixture.client.projects.update({
        projectId: saved.id,
        expectedRevision: saved.revision,
        name: "Later Project update by another client",
      });
      await queryClient.invalidateQueries({ queryKey });
      expect((await read()).find((project) => project.id === created.id)).toMatchObject({
        id: latest.id,
        name: latest.name,
        revision: latest.revision,
      });

      release.resolve();
      await edit;
      expect(queryClient.getQueryData<ProjectSummary[]>(queryKey)).toEqual(
        expect.arrayContaining([
          expect.objectContaining({
            id: latest.id,
            name: latest.name,
            revision: latest.revision,
          }),
        ]),
      );
      await expect(
        commands.update(original, { instructions: "A draft based on the original revision" }),
      ).rejects.toMatchObject({ data: { type: "revision_conflict" } });
      await expect(fixture.client.projects.get(created.id)).resolves.toEqual(latest);
    } finally {
      release.resolve();
      await edit?.catch(() => {});
      queryClient.removeQueries({ queryKey });
      await editing.close();
    }
  }, 30_000);

  it("rereads the open Project catalog before returning a created identity", async () => {
    const queryKey = [PROJECTS_KEY];
    const observer = new QueryObserver(queryClient, {
      queryKey,
      queryFn: ({ signal }) => readRuntimeProjectSummaries(fixture.client, signal),
    });
    const unsubscribe = observer.subscribe(() => {});
    try {
      await observer.refetch({ throwOnError: true });
      const commands = createRuntimeProjectCommands(() => fixture.client);
      const id = await commands.create("New Project from the real command binding");
      const saved = await fixture.client.projects.get(id);
      expect(observer.getCurrentResult().data?.find((project) => project.id === id)).toMatchObject({
        id,
        name: saved.name,
        revision: saved.revision,
      });
    } finally {
      unsubscribe();
      queryClient.removeQueries({ queryKey });
    }
  }, 30_000);
});
