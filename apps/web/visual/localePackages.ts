import { definePlugin } from "@/extensions/definePlugin";
import { createLocaleReconciler } from "@/features/i18n/public/locales";

const declarations = import.meta.glob<{
  contributes: { locales: { tag: string; title: string; entry: string }[] };
}>("../../../plugins/locale-*/package/io.github.tangerg.flame/declaration.json", {
  eager: true,
  import: "default",
});
const dictionaries = import.meta.glob<Record<string, string>>(
  "../../../plugins/locale-*/package/io.github.tangerg.flame/messages.json",
  { import: "default" },
);

export const visualLocalePackages = definePlugin<{}, {}>({
  name: "test.installed-locales",
  async setup(scope) {
    const reconcile = createLocaleReconciler(scope.lifetime("locale-packages"), (error) => {
      throw error;
    });
    await reconcile(
      Object.entries(declarations).flatMap(([path, declaration]) =>
        declaration.contributes.locales.map((locale) => ({
          key: path,
          tag: locale.tag,
          title: locale.title,
          async load(signal: AbortSignal) {
            const entry =
              path.slice(0, path.lastIndexOf("/io.github.tangerg.flame/")) + "/" + locale.entry;
            const read = dictionaries[entry];
            if (!read) throw new Error(`locale catalog is unavailable: ${entry}`);
            const dictionary = await read();
            signal.throwIfAborted();
            return dictionary;
          },
        })),
      ),
    );
  },
});
