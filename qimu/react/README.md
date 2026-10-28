# Qimu React renderer

`@qimu/react` renders a same-Realm native React component acquired through a
Qimu Use. It is optional; the SDK and Browser Runtime do not depend on React.

```ts
const mounted = await mount(
  approvedUse,
  componentObject,
  componentContract,
  container,
  initialProps,
  reportRenderFailure,
);
await mounted.render(nextProps);
await mounted.close();
```

The contract may be a Service or a Point value whose native value is the
component. The consumer owns its props, metadata policy, selection, and renderer
choice. Portable objects cannot be passed to this renderer as native components.

The React root is a child of the original Use Lifetime. Unmounting that root can
finish while sibling asynchronous component work drains. The Use keeps its
provider dependencies until every child and task has actually released. Render
failure requests closure of that original owner. `close` and `wait` retain the
same exit identity; the renderer does not add another shutdown state machine.

Components obtain the explicit Use Lifetime with `useLifetime()`. Register work
with `lifetime.run` and resources with `lifetime.cleanup` before acquisition.
There is no ambient Runtime, provider lookup, or global service container.

`useCurrentUse()` exposes the explicit parent Use to a consumer renderer. A nested
acquisition submits that original ID to the Host; the hook does not approve a
dependency or acquire another provider. The caller joins acquisition through the
parent Lifetime and closes late arrivals. The optional final `wrap` argument to
`mount` reuses application presentation providers in the separately owned React
root. It must not start another plugin Host or copy resource ownership.

The caller must own asynchronous mounting. If mounting finishes after its UI
scope retires, close the returned mount. A terminal mount releases its React root
reference; it cannot render again. React does not unload the evaluated ESM module.
The Browser Runtime separately controls release evidence and any Realm reload.

Run `npm run check --workspace @qimu/react` from the monorepo root.
