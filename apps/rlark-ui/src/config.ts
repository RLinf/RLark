export type DataMode = "mock" | "backend";

function resolveDataMode(): DataMode {
  const configured = String(import.meta.env.VITE_DATA_MODE ?? "")
    .trim()
    .toLowerCase();
  if (configured === "mock" || configured === "backend") return configured;
  return import.meta.env.DEV ? "mock" : "backend";
}

export const dataMode = resolveDataMode();
export const isMockMode = dataMode === "mock";
