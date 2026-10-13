import { useLayoutEffect, useRef, useState } from "react";
import { parseReviewedJSON } from "@flame/runtime-contract/client/json";
import { checkRequest } from "@flame/runtime-contract/client/request";
import type {
  PluginInstallation,
  PluginPoint,
  PluginPointBinding,
  ResolvePluginPointRequest,
} from "@flame/runtime-contract/wire";
import { Button, Metadata, TextField, Toolbar } from "@flame/ui";
import { TextArea } from "@flame/ui/text-field";
import { SystemMessage } from "@/ui";
import { useT } from "@/lib/i18n";
import type { PackageOperations } from "../application/packages";
import { pointMetadata } from "../adapters/pointContracts";

type Read = {
  operations: PackageOperations;
  accepted?: { request: ResolvePluginPointRequest; bindings: PluginPointBinding[] };
  pending: boolean;
  error?: string;
};

export function PointInspection({
  installation,
  operations,
}: {
  installation: PluginInstallation;
  operations: PackageOperations;
}) {
  return installation.selected.points.map((point) => (
    <InspectPoint
      key={`${installation.selected.digest}/${point.id}`}
      installation={installation}
      point={point}
      operations={operations}
    />
  ));
}

function InspectPoint({
  installation,
  point,
  operations,
}: {
  installation: PluginInstallation;
  point: PluginPoint;
  operations: PackageOperations;
}) {
  const t = useT();
  const metadata = pointMetadata(point.contract);
  const [reference, setReference] = useState(
    metadata.scope === "installation" ? installation.id : "",
  );
  const [attributes, setAttributes] = useState("{}");
  const [order, setOrder] = useState("[]");
  const [read, setRead] = useState<Read>();
  const controller = useRef<{ operations: PackageOperations; value: AbortController }>(undefined);
  useLayoutEffect(
    () => () => {
      if (controller.current?.operations === operations) controller.current.value.abort();
    },
    [operations],
  );
  const current = read?.operations === operations ? read : undefined;

  async function inspect() {
    controller.current?.value.abort();
    const owned = new AbortController();
    controller.current = { operations, value: owned };
    const signal = AbortSignal.any([owned.signal, operations.signal]);
    if (signal.aborted) return;
    setRead((previous) => ({
      operations,
      accepted: previous?.operations === operations ? previous.accepted : undefined,
      pending: true,
    }));
    try {
      const request = checkRequest("plugins.resolvePoint", {
        contract: point.contract,
        target: { scope: metadata.scope, reference },
        attributes: parseReviewedJSON(attributes),
        order: parseReviewedJSON(order),
      });
      const result = await operations.track(operations.resolvePoint(request, signal));
      if (!signal.aborted)
        setRead({ operations, accepted: { request, bindings: result.data }, pending: false });
    } catch (error) {
      if (!signal.aborted)
        setRead((previous) => ({
          operations,
          accepted: previous?.operations === operations ? previous.accepted : undefined,
          pending: false,
          error: error instanceof Error ? error.message : t("packages.points.failed"),
        }));
    }
  }

  return (
    <section aria-label={`${point.id} · ${t("packages.points.title")}`}>
      <Toolbar label={t("packages.points.title")}>
        <h4>
          {point.id} · {t("packages.points.title")}
        </h4>
        <Button
          disabled={current?.pending || !reference || operations.signal.aborted}
          onClick={() => void inspect()}
        >
          {t("packages.points.inspect")}
        </Button>
      </Toolbar>
      <Metadata>
        {metadata.reference} · {metadata.composition} · {metadata.scope} · {metadata.ordering}
      </Metadata>
      <Metadata>
        {t(metadata.optional ? "packages.requirements.optional" : "packages.requirements.required")}
      </Metadata>
      <Metadata>{t("packages.points.readOnly")}</Metadata>
      <TextField
        aria-label={t("packages.points.target")}
        placeholder={t("packages.points.target")}
        value={reference}
        onChange={(event) => setReference(event.target.value)}
      />
      <Metadata>{t("packages.points.attributes")}</Metadata>
      <TextArea
        rows={3}
        font="mono"
        aria-label={t("packages.points.attributes")}
        value={attributes}
        onChange={(event) => setAttributes(event.target.value)}
      />
      <Metadata>{t("packages.points.order")}</Metadata>
      <TextArea
        rows={3}
        font="mono"
        aria-label={t("packages.points.order")}
        value={order}
        onChange={(event) => setOrder(event.target.value)}
      />
      {current?.error && <SystemMessage variant="warning">{current.error}</SystemMessage>}
      {current?.accepted ? (
        <>
          <Metadata>{t("packages.points.lastInspected")}</Metadata>
          <pre>
            {JSON.stringify(
              {
                target: current.accepted.request.target,
                attributes: current.accepted.request.attributes,
                order: current.accepted.request.order,
                bindings: current.accepted.bindings,
              },
              null,
              2,
            )}
          </pre>
        </>
      ) : (
        <Metadata>{t("packages.requirements.unread")}</Metadata>
      )}
    </section>
  );
}
