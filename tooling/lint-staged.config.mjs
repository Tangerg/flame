import { getFileInfo } from "prettier";

const ignorePath = new URL("../.prettierignore", import.meta.url);

export default {
  "*.{ts,tsx,mjs,js,json,md,css,html,yml,yaml}": async (files) => {
    const ignored = await Promise.all(
      files.map(
        async (file) => (await getFileInfo(file, { ignorePath })).ignored,
      ),
    );
    const maintained = files.filter((_, index) => !ignored[index]);
    return maintained.length
      ? [
          `prettier --write ${maintained.map((file) => JSON.stringify(file)).join(" ")}`,
          "npm run lint --workspaces --if-present",
        ]
      : [];
  },
};
