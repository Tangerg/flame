import { rolldown } from "rolldown";
import { fileURLToPath } from "node:url";
import { buildComponent } from "@flame/component-sdk/build";

await buildComponent({
  compiler: rolldown,
  input: fileURLToPath(new URL("./component/component.tsx", import.meta.url)),
  directory: fileURLToPath(
    new URL("./package/io.github.tangerg.flame/components", import.meta.url),
  ),
});
