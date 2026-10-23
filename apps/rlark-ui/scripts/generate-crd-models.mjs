import { readFile, readdir, rm, writeFile } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import path from "node:path";
import { generate } from "@kubernetes-models/crd-generate";

const uiDir = path.resolve(path.dirname(fileURLToPath(import.meta.url)), "..");
const repositoryDir = path.resolve(uiDir, "../..");
const crdDir = path.join(repositoryDir, "api/config/crd/bases");
const outputDir = path.join(uiDir, "src/generated/rlark");
const generatedHeader =
  "// Code generated from api/config/crd/bases. DO NOT EDIT.\n\n";
const expectedKinds = [
  "Addon",
  "Cluster",
  "Domain",
  "DomainPeer",
  "Job",
  "Node",
  "Pod",
  "Task",
  "Workflow",
];

const crdFiles = (await readdir(crdDir))
  .filter((name) => name.endsWith(".yaml"))
  .sort();

if (crdFiles.length === 0) {
  throw new Error(`no CRD manifests found in ${crdDir}`);
}

const documents = await Promise.all(
  crdFiles.map((name) => readFile(path.join(crdDir, name), "utf8")),
);

await rm(outputDir, { recursive: true, force: true });
await generate({
  input: documents.join("\n---\n"),
  outputPath: outputDir,
  yamlVersion: "1.2",
});

async function addGeneratedHeader(directory) {
  const entries = await readdir(directory, { withFileTypes: true });
  await Promise.all(
    entries.map(async (entry) => {
      const file = path.join(directory, entry.name);
      if (entry.isDirectory()) {
        await addGeneratedHeader(file);
        return;
      }
      if (!entry.name.endsWith(".ts") && !entry.name.endsWith(".js")) return;
      const contents = await readFile(file, "utf8");
      const normalized = contents.replace(/[\t ]+$/gm, "");
      await writeFile(file, `${generatedHeader}${normalized}`);
    }),
  );
}

await addGeneratedHeader(outputDir);

const versionDir = path.join(outputDir, "rlinf.io/v1alpha1");
const generatedKinds = (await readdir(versionDir))
  .filter((name) => name.endsWith(".ts") && name !== "index.ts")
  .map((name) => path.basename(name, ".ts"))
  .sort();

if (generatedKinds.join("\n") !== expectedKinds.sort().join("\n")) {
  throw new Error(
    `generated CRD kinds do not match the registered RLark kinds: ${generatedKinds.join(", ")}`,
  );
}
