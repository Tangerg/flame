import {
  addInstalledMemory,
  captureMemoryBinding,
  deleteInstalledMemory,
  readInstalledMemory,
} from "./memoryOperations";
import { readFile } from "node:fs/promises";
import { join } from "node:path";
import { describe, expect, it, onTestFinished } from "vitest";
import { componentExport } from "@flame/component-sdk/runtime-profile";
import { defineUnit, FrameError, Lifetime, Service } from "@frame/sdk";
import { ControlPump, HTTPControl, Runtime } from "@frame/sdk/browser";
import { RpcError } from "@flame/runtime-client";
import { useRuntimeFixture } from "./fixture";

describe("Go Runtime ↔ HTTP ↔ TypeScript SDK: plugins", () => {
  const fixture = useRuntimeFixture("plugins");

  it("admits the shipped portable package through the shared client and revokes its reads", async () => {
    await expect(fixture.client.plugins.install({ source: "relative/package" })).rejects.toSatisfy(
      (error: unknown) => error instanceof RpcError && error.data.type === "invalid_params",
    );
    const defaultTrajectory = (await fixture.client.plugins.list()).data.find(
      (item) => item.selected.name === "flame.trajectory",
    )!;
    await fixture.client.plugins.setEnablement({
      installationId: defaultTrajectory.id,
      enabled: false,
    });
    const installed = await fixture.client.plugins.install({
      source: join(fixture.environmentRoot, "plugins", "trajectory"),
    });
    const target = { installationId: installed.id, digest: installed.selected.digest };
    const reader = fixture.createSharedClient();
    try {
      expect(installed.enabled).toBe(false);
      expect(installed.selected.diagnostics).toEqual([]);
      expect(installed.selected.themes).toHaveLength(1);
      expect(installed.selected.skills).toHaveLength(1);
      await fixture.client.plugins.setEnablement({ installationId: installed.id, enabled: true });
      const component = installed.selected.components.find((value) => value.id === "trajectory")!;
      expect(component.scope).toBe("session");
      const effect = installed.selected.components.find(
        (value) => value.placement === "clientEffect",
      )!;
      expect(effect).toMatchObject({ scope: "client", export: "SessionExports" });
      const resourceRequest = { ...target, componentId: component.id, path: component.entry };
      const resource = await fixture.client.plugins.readResource(resourceRequest);
      expect(Buffer.from(resource.body, "base64")).toEqual(
        await readFile(join(fixture.environmentRoot, "plugins", "trajectory", component.entry)),
      );
      await expect(
        fixture.client.plugins.readResource({
          ...target,
          componentId: effect.id,
          path: effect.entry,
        }),
      ).resolves.toEqual(resource);
      const session = await fixture.client.sessions.create({
        workspace: { path: fixture.root },
        title: "Package evidence",
      });
      const query = { sessionId: session.id, includeDescendants: true, limit: 1 };
      const trajectory = await reader.sessions.trajectory(query);
      expect(trajectory.data).toEqual([]);
      const skills = await fixture.client.workspace({ path: fixture.root }).skills.listDiscovered();
      expect(skills.skills).toContainEqual(
        expect.objectContaining({
          name: "inspect-history",
          scope: "installation",
          installation: expect.objectContaining({
            installationId: installed.id,
            digest: target.digest,
          }),
        }),
      );
      await fixture.client.plugins.setEnablement({ installationId: installed.id, enabled: false });
      await expect(fixture.client.plugins.readResource(resourceRequest)).rejects.toMatchObject({
        code: -32042,
      });
      expect(await reader.sessions.trajectory(query)).toEqual(trajectory);
      const revokedSkills = await fixture.client
        .workspace({ path: fixture.root })
        .skills.listDiscovered();
      expect(
        revokedSkills.skills.some((skill) => skill.installation?.installationId === installed.id),
      ).toBe(false);
    } finally {
      await reader.close();
      await fixture.client.plugins.uninstall(installed.id);
      await fixture.client.plugins.setEnablement({
        installationId: defaultTrajectory.id,
        enabled: true,
      });
    }
  }, 30_000);

  it("reads only the exact enabled installed locale catalog without executable contributions", async () => {
    const installation = (await fixture.client.plugins.list()).data.find(
      (value) => value.selected.name === "flame.locale.ja",
    );
    if (!installation) throw new Error("the shipped Japanese locale is unavailable");
    expect(installation.selected.locales).toEqual([{ tag: "ja", title: "日本語" }]);
    expect(installation.selected.components).toEqual([]);
    expect(installation.selected.servers).toEqual([]);
    expect(installation.selected.contributions).toEqual([]);
    const request = {
      installationId: installation.id,
      digest: installation.selected.digest,
      tag: "ja",
    };
    const messages = JSON.parse(
      await readFile(
        join(
          fixture.environmentRoot,
          "plugins",
          "locale-ja",
          "io.github.tangerg.flame/messages.json",
        ),
        "utf8",
      ),
    );
    await expect(fixture.client.plugins.readLocale(request)).resolves.toEqual({ messages });
    await expect(
      fixture.client.plugins.readLocale({ ...request, digest: "f".repeat(64) }),
    ).rejects.toMatchObject({
      data: { type: "plugin_stale" },
    });
    await expect(
      fixture.client.plugins.readLocale({ ...request, tag: "ko" }),
    ).rejects.toMatchObject({
      data: { type: "plugin_not_found" },
    });
    await fixture.client.plugins.setEnablement({ installationId: installation.id, enabled: false });
    try {
      await expect(fixture.client.plugins.readLocale(request)).rejects.toMatchObject({
        data: { type: "plugin_disabled" },
      });
    } finally {
      await fixture.client.plugins.setEnablement({
        installationId: installation.id,
        enabled: true,
      });
    }
    await expect(fixture.client.plugins.readLocale(request)).resolves.toEqual({ messages });
  }, 30_000);

  it("admits plugins after SIGKILL while a vanished window still holds a component", async () => {
    const installation = (await fixture.client.plugins.list()).data.find(
      (item) => item.selected.name === "flame.trajectory",
    )!;
    const entry = installation.selected.components.find((value) => value.id === "trajectory")!;
    const session = await fixture.client.plugins.openBrowserRealm({
      operationId: crypto.randomUUID(),
      resumeToken: crypto.randomUUID() + crypto.randomUUID(),
    });
    const windowLifetime = new Lifetime("vanished-window");
    const contract = new Service(session.componentContract);
    const realm = new Runtime({
      authority: session.authority,
      realm: session.realm,
      proofCapacity: session.proofCapacity,
      control: new HTTPControl({
        endpoint: new URL(session.controlEndpoint, fixture.baseUrl).href,
        credential: session.credential,
        maximumMessageBytes: session.maximumMessageBytes,
        lifetime: windowLifetime.branch("control"),
      }),
      load: (unit) =>
        defineUnit({
          id: unit.instance.source.unitId,
          imports: {},
          exports: { [componentExport]: contract },
          setup: () => ({ [componentExport]: () => null }),
        }),
      remote: () => {
        throw new FrameError("contract_mismatch", "components declare no portable imports");
      },
    });
    const pumping = new AbortController();
    onTestFinished(() => pumping.abort());
    void new ControlPump(realm, {
      intervalMilliseconds: session.pollIntervalMilliseconds,
      preparationCapacity: session.preparationCapacity,
      onError: () => {},
    })
      .run(pumping.signal)
      .catch(() => {});
    const grant = await fixture.client.plugins.acquireComponent({
      installationId: installation.id,
      digest: installation.selected.digest,
      componentId: entry.id,
      operationId: crypto.randomUUID(),
      realm: session.realm,
    });
    await realm.accept(grant);
    // A closed or reloaded window sends no exit proof for the component it held.
    windowLifetime.beginClose();

    await fixture.client.close();
    fixture.client = undefined;
    await fixture.killRuntimeProcess();
    await fixture.startRuntimeProcess();
    fixture.client = fixture.createRuntimeClient();
    fixture.negotiated = (await fixture.client.runtime.discover()).capabilities;
    await fixture.awaitManagedOwners();

    const readiness = await fetch(`${fixture.baseUrl}/v2/health/ready`);
    expect(await readiness.json()).toMatchObject({ checks: { plugins: "ok" } });
    const restored = (await fixture.client.plugins.list()).data.find(
      (item) => item.id === installation.id,
    );
    expect(restored?.presentation).toBe("admitted");
  }, 60_000);

  it("reads the shipped Memory backend page and resumes private facts after withdrawal", async () => {
    const installation = fixture.managedOwners.get("memory")!;
    const binding = await captureMemoryBinding(fixture.client);
    const release = { installationId: installation.id, digest: installation.selected.digest };
    const component = installation.selected.components[0]!;
    const resourceRequest = { ...release, componentId: component.id, path: component.entry };
    const workspace = { path: fixture.root };
    let created: Awaited<ReturnType<typeof addInstalledMemory>> | undefined;
    let withdrawn = false;
    try {
      expect(installation.selected.diagnostics).toEqual([]);
      created = await addInstalledMemory(fixture.client, binding, {
        scope: "project",
        cwd: workspace.path,
        content: "Memory page canonical ownership fixture",
      });
      const query = { scope: "project" as const, workspace, limit: 1 };
      expect(
        (
          await readInstalledMemory(fixture.client, binding, "project", query.workspace.path).then(
            (data) => ({
              data,
            }),
          )
        ).data,
      ).toContainEqual(created);

      expect(component.scope).toBe("client");
      expect(
        Buffer.from((await fixture.client.plugins.readResource(resourceRequest)).body, "base64"),
      ).toEqual(
        await readFile(join(fixture.environmentRoot, "plugins", "memory", component.entry)),
      );
      await fixture.client.plugins.setEnablement({
        installationId: installation.id,
        enabled: false,
      });
      withdrawn = true;
      await expect(fixture.client.plugins.readResource(resourceRequest)).rejects.toMatchObject({
        code: -32042,
      });
      await expect(
        readInstalledMemory(fixture.client, binding, "project", query.workspace.path).then(
          (data) => ({
            data,
          }),
        ),
      ).rejects.toMatchObject({ data: { type: "plugin_disabled" } });
      await fixture.client.plugins.setEnablement({
        installationId: installation.id,
        enabled: true,
      });
      await fixture.awaitMCPTools(
        { origin: { type: "installation", installationId: installation.id }, name: "memory" },
        (tools) => tools.some((tool) => tool.name === "search_memory"),
      );
      withdrawn = false;
      expect(
        await readInstalledMemory(fixture.client, binding, "project", workspace.path),
      ).toContainEqual(created);
    } finally {
      if (withdrawn) await fixture.activateManagedOwner("memory");
      if (created)
        await deleteInstalledMemory(fixture.client, binding, created.id, created.revision);
      await fixture.client.plugins.uninstall(installation.id);
      fixture.managedOwners.delete("memory");
    }
  }, 30_000);
});
