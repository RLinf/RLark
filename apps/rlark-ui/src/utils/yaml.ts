import { stringify } from "yaml";

// Kubernetes tooling (kubectl, sigs.k8s.io/yaml) reads YAML 1.1, where plain
// scalars such as "yes", "1_000", "1:2:3", and "2026-11-23" resolve to a
// non-string type. Serializing against the same version is what keeps a string
// the form captured from round-tripping into a bool, int, or timestamp.
const YAML_VERSION = "1.1";

function isPlainObject(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

// The form models unset fields as undefined, null, "", or an empty map, and a
// preview that echoed them back would suggest the server receives them. Drop
// those map entries, including a parent left empty only because every child was
// dropped. Sequence entries are positional, so they are kept as authored.
function prune(value: unknown): unknown {
  if (Array.isArray(value)) return value.map(prune);
  if (!isPlainObject(value)) return value;
  const kept: Record<string, unknown> = {};
  for (const [key, raw] of Object.entries(value)) {
    const child = prune(raw);
    if (child === undefined || child === null || child === "") continue;
    if (isPlainObject(child) && Object.keys(child).length === 0) continue;
    kept[key] = child;
  }
  return kept;
}

// The trailing newline the library emits is kept deliberately: stripping it
// would re-chomp a block scalar whose value ends in a blank line.
export function toYaml(obj: unknown): string {
  return stringify(prune(obj), { version: YAML_VERSION });
}
