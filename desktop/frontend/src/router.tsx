import { useSyncExternalStore } from "react";
import {
  createBrowserHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { lookupExtensionPoint, ROUTE } from "@/plugins/sdk";
import {
  applyPatch,
  configureNavigator,
  sameLocation,
  type AppLocation,
  type Navigator,
} from "@/lib/navigation";
import { Devtools } from "@/Devtools";

interface AppSearch {
  session?: string;
  view?: string;
  dock?: string;
  settings?: string;
}

function locationFrom(read: (key: string) => unknown): AppLocation {
  const param = (key: string): string | null => {
    const value = read(key);
    return typeof value === "string" && value.length > 0 ? value : null;
  };
  return {
    session: param("session") ?? "",
    view: param("view"),
    dock: param("dock"),
    settings: param("settings"),
  };
}

const history = createBrowserHistory();

function readLocation(): AppLocation {
  const params = new URLSearchParams(history.location.search);
  return locationFrom((key) => params.get(key));
}

function searchOf(location: AppLocation): AppSearch {
  return {
    session: location.session || undefined,
    view: location.view ?? undefined,
    dock: location.dock ?? undefined,
    settings: location.settings ?? undefined,
  };
}

function hrefOf(location: AppLocation): string {
  const params = new URLSearchParams();
  for (const [key, value] of Object.entries(searchOf(location))) {
    if (value !== undefined) params.set(key, value);
  }
  const query = params.toString();
  return query ? `${history.location.pathname}?${query}` : history.location.pathname;
}

const historyNavigator: Navigator = {
  get: readLocation,
  use: (select) =>
    useSyncExternalStore(
      (onChange) => history.subscribe(onChange),
      () => select(readLocation()),
    ),
  subscribe(listener) {
    let previous = readLocation();
    return history.subscribe(() => {
      const next = readLocation();
      if (sameLocation(previous, next)) return;
      const before = previous;
      previous = next;
      listener(next, before);
    });
  },
  go(patch, options) {
    const current = readLocation();
    const next = applyPatch(current, patch);
    if (sameLocation(current, next)) return;
    const href = hrefOf(next);
    if (options?.replace === true) history.replace(href);
    else history.push(href);
  },
  back: () => history.back(),
  forward: () => history.forward(),
};

configureNavigator(historyNavigator);

const rootRoute = createRootRoute({
  validateSearch: (search: Record<string, unknown>): AppSearch =>
    searchOf(locationFrom((key) => search[key])),
  component: () => <Outlet />,
});

function buildRouter() {
  const specs = lookupExtensionPoint(ROUTE);
  const routes = specs.map((spec) =>
    createRoute({
      getParentRoute: () => rootRoute,
      path: spec.path,
      component: spec.component as Parameters<typeof createRoute>[0]["component"],
    }),
  );
  return createRouter({
    routeTree: rootRoute.addChildren(routes),
    defaultPreload: "intent",
    history,
  });
}

declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<typeof buildRouter>;
  }
}

let instance: ReturnType<typeof buildRouter> | null = null;

function appRouter(): ReturnType<typeof buildRouter> {
  return (instance ??= buildRouter());
}

export function AppRouter() {
  const router = appRouter();
  return (
    <>
      <RouterProvider router={router} />
      <Devtools router={router} />
    </>
  );
}
