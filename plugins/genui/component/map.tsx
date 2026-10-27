import {
  createContext,
  useCallback,
  useContext,
  useLayoutEffect,
  useMemo,
  useState,
} from "react";
import {
  Map as PigeonMap,
  Overlay,
  GeoJson,
  type PigeonProps,
  type TileComponentProps,
} from "pigeon-maps";
import {
  Button,
  Metadata,
  RecordGroup,
  ScrollRegion,
  Toolbar,
} from "@flame/ui";
import type { Translate } from "@flame/component-sdk";
import {
  useGetFieldValue,
  type ComponentRenderProps,
} from "@openuidev/react-lang";
import type { DisplayProps } from "../contracts/catalog";
import { stateKey } from "../contracts/document";
import { useGenUIInteraction } from "./interaction";

type TileStatus = "loading" | "ready" | "failed";
const TileResources = createContext<{
  retry: number;
  report(key: string, status?: TileStatus): void;
} | null>(null);

function TileView({ tile, tileLoaded }: TileComponentProps) {
  const resource = useContext(TileResources)!;
  const { report, retry } = resource;
  useLayoutEffect(() => {
    if (tile.active) report(tile.key, "loading");
    else report(tile.key);
    return () => report(tile.key);
  }, [tile.key, tile.active, retry, report]);
  return (
    <img
      key={retry}
      src={tile.url}
      srcSet={tile.srcSet}
      width={tile.width}
      height={tile.height}
      alt=""
      onLoad={() => {
        if (tile.active) report(tile.key, "ready");
        tileLoaded();
      }}
      onError={() => {
        if (tile.active) report(tile.key, "failed");
        tileLoaded();
      }}
      style={{ position: "absolute", left: tile.left, top: tile.top }}
    />
  );
}

function MapControls({
  mapState,
  mapProps,
  setCenterZoom,
  t,
}: PigeonProps & { t: Translate }) {
  if (!mapState || !mapProps || !setCenterZoom)
    throw new Error("genui: map controls require map context");
  return (
    <div
      className="pigeon-drag-block"
      style={{ position: "absolute", top: 8, left: 8 }}
    >
      <Toolbar label={t("map.controls")}>
        <Button
          variant="outline"
          aria-label={t("map.zoomIn")}
          disabled={mapState.zoom >= (mapProps.maxZoom ?? 18)}
          onClick={() =>
            setCenterZoom(
              mapState.center,
              Math.min(mapState.zoom + 1, mapProps.maxZoom ?? 18),
            )
          }
        >
          +
        </Button>
        <Button
          variant="outline"
          aria-label={t("map.zoomOut")}
          disabled={mapState.zoom <= (mapProps.minZoom ?? 1)}
          onClick={() =>
            setCenterZoom(
              mapState.center,
              Math.max(mapState.zoom - 1, mapProps.minZoom ?? 1),
            )
          }
        >
          −
        </Button>
      </Toolbar>
    </div>
  );
}

export function MapView({
  props,
  t,
}: ComponentRenderProps<DisplayProps["Map"]> & { t: Translate }) {
  const read = useGetFieldValue();
  const { update, readInitial } = useGenUIInteraction();
  const selected = props.selectedState
    ? ((read(undefined, stateKey(props.selectedState)) ??
        readInitial(props.selectedState)) as string)
    : undefined;
  const [resources, setResources] = useState<ReadonlyMap<string, TileStatus>>(
    new Map(),
  );
  const [retry, setRetry] = useState(0);
  const report = useCallback((key: string, status?: TileStatus) => {
    setResources((current) => {
      if (current.get(key) === status) return current;
      const next = new Map(current);
      if (status === undefined) next.delete(key);
      else next.set(key, status);
      return next;
    });
  }, []);
  const resource = useMemo(() => ({ retry, report }), [retry, report]);
  const failures = [...resources.values()].filter(
    (status) => status === "failed",
  ).length;
  const loading = [...resources.values()].some(
    (status) => status === "loading",
  );
  const center = useMemo<[number, number]>(
    () =>
      props.markers.length
        ? [
            props.markers.reduce((sum, marker) => sum + marker.latitude, 0) /
              props.markers.length,
            props.markers.reduce((sum, marker) => sum + marker.longitude, 0) /
              props.markers.length,
          ]
        : [0, 0],
    [props.markers],
  );
  const choose = (id: string) => {
    if (props.selectedState) update(props.selectedState, id);
  };

  return (
    <RecordGroup aria-label={props.caption}>
      <span>{props.caption}</span>
      <TileResources value={resource}>
        <PigeonMap
          defaultCenter={center}
          defaultZoom={props.markers.length === 1 ? 10 : 4}
          height={320}
          defaultWidth={640}
          animate={false}
          metaWheelZoom
          metaWheelZoomWarning={t("map.wheel")}
          twoFingerDrag
          twoFingerDragWarning={t("map.touch")}
          tileComponent={TileView}
        >
          <MapControls t={t} />
          {props.route && props.route.length > 1 && (
            <GeoJson
              data={{
                type: "FeatureCollection",
                features: [
                  {
                    type: "Feature",
                    properties: {},
                    geometry: {
                      type: "LineString",
                      coordinates: props.route.map((point) => [
                        point.longitude,
                        point.latitude,
                      ]),
                    },
                  },
                ],
              }}
              svgAttributes={{
                fill: "none",
                stroke: "currentColor",
                strokeWidth: 3,
              }}
            />
          )}
          {props.markers.map((marker, index) => (
            <Overlay
              key={marker.id}
              anchor={[marker.latitude, marker.longitude]}
              offset={[16, 16]}
            >
              {props.selectedState ? (
                <Button
                  variant={selected === marker.id ? "primary" : "outline"}
                  round
                  size="icon-sm"
                  aria-label={marker.label}
                  aria-pressed={selected === marker.id}
                  onClick={() => choose(marker.id)}
                >
                  {index + 1}
                </Button>
              ) : (
                <a
                  href={`https://www.openstreetmap.org/?mlat=${marker.latitude}&mlon=${marker.longitude}`}
                  target="_blank"
                  rel="noreferrer noopener"
                  aria-label={marker.label}
                >
                  {index + 1}
                </a>
              )}
            </Overlay>
          ))}
        </PigeonMap>
      </TileResources>
      {failures > 0 ? (
        <Toolbar label={t("map.loadingError")}>
          <Metadata role="alert">{t("map.loadingError")}</Metadata>
          <Button onClick={() => setRetry((value) => value + 1)}>
            {t("retry")}
          </Button>
        </Toolbar>
      ) : loading ? (
        <Metadata role="status">{t("map.loading")}</Metadata>
      ) : null}
      <ScrollRegion label={t("map.locations", { map: props.caption })}>
        <table>
          <caption style={{ textAlign: "start" }}>
            {t("map.locations", { map: props.caption })}
          </caption>
          <thead>
            <tr>
              <th scope="col">{t("map.place")}</th>
              <th scope="col">{t("map.coordinates")}</th>
              <th scope="col">{t("map.description")}</th>
            </tr>
          </thead>
          <tbody>
            {props.markers.map((marker) => (
              <tr
                key={marker.id}
                aria-selected={
                  props.selectedState ? selected === marker.id : undefined
                }
              >
                <th scope="row">
                  {props.selectedState ? (
                    <Button
                      variant="bare"
                      active={selected === marker.id}
                      onClick={() => choose(marker.id)}
                    >
                      {marker.label}
                    </Button>
                  ) : (
                    marker.label
                  )}
                </th>
                <td>
                  {marker.latitude}, {marker.longitude}
                </td>
                <td>{marker.description}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </ScrollRegion>
    </RecordGroup>
  );
}
