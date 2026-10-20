const generatedOrLiteral = (file) =>
  /(?:\.generated\.|\/generated\/|\/runtime\/contract\/|\/package\/.*\/(?:components|contracts)\/)/.test(
    file,
  );
export default {
  "*.{ts,tsx,mjs,js,json,md,css,html,yml,yaml}": (files) => {
    const maintained = files.filter((file) => !generatedOrLiteral(file));
    return maintained.length
      ? [
          `prettier --write ${maintained.map((file) => JSON.stringify(file)).join(" ")}`,
          "npm run lint --workspaces --if-present",
        ]
      : [];
  },
};
