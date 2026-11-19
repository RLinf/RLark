import { readFile } from "node:fs/promises";

export async function readStyles() {
  const entryUrl = new URL("../src/styles.css", import.meta.url);
  const entry = await readFile(entryUrl, "utf8");
  const imports = [...entry.matchAll(/@import\s+["'](.+?)["'];/g)];

  return Promise.all(
    imports.map((match) => readFile(new URL(match[1], entryUrl), "utf8")),
  ).then((parts) => parts.join(""));
}
