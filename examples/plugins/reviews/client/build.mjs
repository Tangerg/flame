import { buildComponent } from "@flame/plugin-sdk/build";

await buildComponent({
  input: new URL("board.tsx", import.meta.url).pathname,
  directory: new URL(
    "../package/io.github.tangerg.flame/components",
    import.meta.url,
  ).pathname,
});
