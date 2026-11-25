import { useCallback, useMemo } from "react";
import {
  failureText,
  useContextState,
  type ComponentContext,
} from "@flame/component-sdk";
import { METHODS } from "../contracts/methods";
import type { Asset, Notice } from "./types";
import type { DeliverableOperations } from "./operations";
import { downloadAsset } from "./transfer";

export function useAssetSave(
  context: ComponentContext,
  operations: DeliverableOperations,
  signal: AbortSignal,
  tell: (notice: Notice) => void,
) {
  const [transfer, setTransfer] = useContextState<{
    filename: string;
    received: number;
    total: number;
  }>(context);
  const saving = useMemo(
    () => ({
      active: false,
      controller: undefined as AbortController | undefined,
    }),
    [context],
  );

  const save = useCallback(
    async (asset: Asset) => {
      if (signal.aborted || saving.active) return;
      const controller = new AbortController();
      saving.controller = controller;
      saving.active = true;
      const lifetime = AbortSignal.any([signal, controller.signal]);
      setTransfer({
        filename: asset.filename,
        received: 0,
        total: Number(asset.size),
      });
      try {
        const bytes = await downloadAsset(
          (input, owned) => operations.query(METHODS.download, input, owned),
          asset,
          lifetime,
          (received, total) => {
            if (!lifetime.aborted)
              setTransfer({ filename: asset.filename, received, total });
          },
        );
        lifetime.throwIfAborted();
        const result = await context.saveFile({
          filename: asset.filename,
          mime: asset.mime,
          bytes,
        });
        if (!signal.aborted)
          tell({ kind: "save", result, filename: asset.filename });
      } catch (error) {
        if (!lifetime.aborted)
          tell({ kind: "error", text: failureText(error) });
      } finally {
        saving.active = false;
        saving.controller = undefined;
        if (!signal.aborted) setTransfer(undefined);
      }
    },
    [context, operations, saving, setTransfer, signal, tell],
  );

  return {
    save,
    transfer,
    cancelTransfer: () => saving.controller?.abort(),
  };
}
