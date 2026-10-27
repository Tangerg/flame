# Independent component composition

`consumer` and `provider` are independently installable ordinary packages. Neither
imports application source, the other package's module, or a private host registry.
Installing either first is supported. Their component graph is finite even though
both packages contribute to each other:

- Consumer owns the board and the panels, frames and summary point contracts.
- Provider contributes a panel, two frames and a selectable summary. It owns the
  details point, consumed inside its panel.
- Consumer independently exports an inspector to Provider's details point.

The consumer's Go-independent JSON point files are the contract sources. Provider
owns `details.json`; Consumer owns the other three. `build.mjs` copies each into
the other package as a projection and checks byte equality. Declarations and React
code consume those files; the host has no example-specific branch. Point-value
schemas own contribution data. `PluginComponent<Input, Props>` owns the ordinary
typed React input/props contract. Runtime owns installation and point resolution;
the host owns component resource/render lifetimes.

From the repository root:

```sh
make setup
npm run typecheck --workspace @flame/composition-example
npm run build --workspace @flame/composition-example
npm run check:resources --workspace @flame/composition-example
mkdir -p out/plugins/composition
mkdir out/plugins/composition/consumer out/plugins/composition/provider
cp -R examples/plugins/composition/consumer/package/. out/plugins/composition/consumer/
cp -R examples/plugins/composition/provider/package/. out/plugins/composition/provider/
go run ./runtime/cmd/pluginpack --source "$PWD/out/plugins/composition/consumer"
go run ./runtime/cmd/pluginpack --source "$PWD/out/plugins/composition/provider"
```

Use new output directories for each release. The original `pluginpack` builder
projects the source metadata into Qimu's `manifest.json` and `plugin.lock.json`;
the source directories are not installable releases. These packages contain only
portable component resources, so no native build target is required.

Install the built `out/plugins/composition/consumer` and
`out/plugins/composition/provider` directories with the ordinary Plugins UI or
`plugins install`, then enable them. Open **CompositionBoard**. The board shows the
Provider panel containing Consumer's inspector, wrapped by two actual Provider
frames. **Use contributed summary** selects the independent summary. The board
owns that local selection and explicit frame order; it does not persist another
installation/provider decision. The example creates no backend or model call.

Disable/uninstall withdraws contributions through Runtime. Required or explicitly
selected unavailable contributions fail rather than silently choosing another.
The example's optional points permit absence; its consumer removes unavailable
entries from its own current UI selection/order. Staging leaves current bindings
unchanged; selecting the candidate release changes them. ESM evaluation belongs
to the window: after resource retirement or evaluated-version replacement, reload
when requested to establish the current graph. Uninstalling either package does
not run the other's business operations.

Runtime's public lifecycle test covers both installation orders, reverse
contributions, exact selection/decoration order, disable/reenable, staged versus
selected upgrade, stale/disabled resource refusal, and uninstall. Graphical tests
exercise actual component composition, cancellation/join, late imports, failed
resolution/loading/rendering and cycle detection. Build, Runtime and browser
verification have distinct scopes.
