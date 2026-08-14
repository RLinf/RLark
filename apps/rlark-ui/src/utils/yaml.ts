export function toYaml(obj: unknown, indent = 0): string {
  const pad = " ".repeat(indent);
  if (obj === null || obj === undefined) return "null";
  if (typeof obj === "string")
    return /[:\n#{}\[\],&*?|>=%@`]/.test(obj) || obj === ""
      ? `"${obj.replace(/"/g, '\\"')}"`
      : obj;
  if (typeof obj === "number" || typeof obj === "boolean") return String(obj);
  if (Array.isArray(obj)) {
    if (obj.length === 0) return "[]";
    return obj
      .map((item) => {
        if (item !== null && typeof item === "object") {
          const lines = toYaml(item, indent + 2).split("\n");
          const firstLine = lines[0].replace(/^\s+/, "");
          const restLines = lines.slice(1).join("\n");
          return `${pad}- ${firstLine}${restLines ? "\n" + restLines : ""}`;
        }
        return `${pad}- ${toYaml(item, indent)}`;
      })
      .join("\n");
  }
  if (typeof obj === "object") {
    const entries = Object.entries(obj).filter(
      ([, v]) => v !== undefined && v !== null && v !== "",
    );
    if (entries.length === 0) return "{}";
    return entries
      .map(([key, val]) => {
        if (
          val !== null &&
          typeof val === "object" &&
          !Array.isArray(val) &&
          Object.keys(val).length === 0
        )
          return null;
        if (val !== null && typeof val === "object") {
          const inner = toYaml(val, indent + 2);
          return `${pad}${key}:\n${inner}`;
        }
        return `${pad}${key}: ${toYaml(val, indent)}`;
      })
      .filter(Boolean)
      .join("\n");
  }
  return String(obj);
}
