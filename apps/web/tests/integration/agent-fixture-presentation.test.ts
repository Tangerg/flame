import { projectAgentSessionSnapshot } from "@/features/agent/application/session/sessionSnapshot";
import { expect, it } from "vitest";
import {
  projectRuntimeAgentSnapshot,
  RUNTIME_AGENT_SESSION_SNAPSHOTS,
} from "../../lab/scenarios/agentSessionSnapshots";

const { toolCalls } = projectAgentSessionSnapshot(
  projectRuntimeAgentSnapshot(RUNTIME_AGENT_SESSION_SNAPSHOTS["tool-shells"]),
);

it("projects the captured Apply Patch receipt with its declared renderer shape", () => {
  const tool = toolCalls.item_shells_edit;
  const changes = [
    {
      path: "/Users/visual/scope/apps/web/src/features/chat/tools/application/specialisedPreviewProjections.ts",
      status: "modified",
      added: 0,
      removed: 0,
    },
    {
      path: "runtime/internal/session/atomicity_and_idempotency.go",
      status: "moved",
      from: "runtime/internal/session/store_atomicity.go",
      added: 0,
      removed: 0,
    },
    {
      path: "runtime/internal/session/legacy_store_shim.go",
      status: "deleted",
      added: 0,
      removed: 0,
    },
  ];
  expect(tool).toMatchObject({
    name: "apply_patch",
    status: "ok",
    category: "edit",
    files: 3,
    added: 0,
    removed: 0,
    changes,
  });
  expect(JSON.parse(tool!.result!)).toEqual({ changes });
  expect(tool?.resultShape).toBe("patch");
});

it("leaves the captured Read result without a shared renderer shape", () => {
  const tool = toolCalls.item_shells_read;
  expect(tool).toMatchObject({ name: "read", status: "ok", category: "read" });
  expect(tool).not.toHaveProperty("resultShape");
});
