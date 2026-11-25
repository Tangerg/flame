import { useState, useSyncExternalStore } from "react";
import {
  createBrowserHistory,
  createRootRoute,
  createRoute,
  createRouter,
  Outlet,
  RouterProvider,
} from "@tanstack/react-router";
import { lookupExtensionPoint } from "@/extensions/selectors/extensions";
import { ROUTE } from "@/features/shell/public/routes";
import { applyPatch, sameLocation, type AppLocation, type Navigator } from "@/lib/navigation";
import { Devtools } from "@/Devtools";
interface AppSearch {
  session?: string;
  view?: string;
  dock?: string;
  settings?: string;
  subagent?: string;
  target?: string;
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
    subagent: param("subagent"),
    target: param("target"),
  };
}

export function createAppNavigation() {
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
      subagent: location.subagent ?? undefined,
      target: location.target ?? undefined,
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
    href: (patch) => new URL(hrefOf(applyPatch(readLocation(), patch)), window.location.href).href,
    back: () => history.back(),
    forward: () => history.forward(),
  };

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
      // Every search value is a plain string. The default JSON parsing would
      // turn a JSON-shaped view target or a numeric id into another type.
      parseSearch: (search) => Object.fromEntries(new URLSearchParams(search)),
      stringifySearch: (search) => {
        const query = new URLSearchParams(
          Object.entries(search).filter(
            (entry): entry is [string, string] => typeof entry[1] === "string",
          ),
        ).toString();
        return query ? `?${query}` : "";
      },
    });
  }

  return { navigator: historyNavigator, buildRouter, dispose: () => history.destroy() };
}
export type AppNavigation = ReturnType<typeof createAppNavigation>;
declare module "@tanstack/react-router" {
  interface Register {
    router: ReturnType<AppNavigation["buildRouter"]>;
  }
}
export function AppRouter({ navigation }: { navigation: AppNavigation }) {
  const [router] = useState(() => navigation.buildRouter());
  return (
    <>
      <RouterProvider router={router} />
      <Devtools router={router} />
    </>
  );
}
