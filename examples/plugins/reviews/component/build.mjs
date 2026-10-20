import { rolldown } from "rolldown";
import { buildComponent } from "@flame/component-sdk/build";

await buildComponent({
  compiler: rolldown,
  input: new URL("board.tsx", import.meta.url).pathname,
  directory: new URL(
    "../package/io.github.tangerg.flame/components",
    import.meta.url,
  ).pathname,
});
