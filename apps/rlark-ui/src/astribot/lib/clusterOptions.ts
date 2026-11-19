export type RegistryClusterOption = { id: string; name: string };

// Registry delivery uses the historical namespace suffix, never a display name.
export function registryClusterOption(cluster: {
  id: string;
  name?: string;
}): RegistryClusterOption {
  return {
    id: cluster.id.replace(/^rlark-/, ""),
    name: cluster.name || cluster.id,
  };
}
