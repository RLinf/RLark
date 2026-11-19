import type { StorageClass, StorageProvider, StorageType } from "../../data";

export type StorageClassFormState = {
  name: string;
  clusters: string[];
  type: StorageType;
  provider: StorageProvider;
  endpoint: string;
  mount_endpoint: string;
  cluster_mount_endpoints: Record<string, string>;
  prefix: string;
  vfs_cache_mode: "off" | "writes";
  mount_profile: "default" | "read" | "archive";
  mount_options: string[];
  region: string;
  bucket: string;
  access_key_id: string;
  access_key_secret: string;
  path_style: boolean;
  host: string;
  share: string;
  sub_path: string;
  username: string;
  password: string;
  domain: string;
  path: string;
  /** The file system type the directory must be on, as /proc/mounts lists it. */
  fs_type: string;
  description: string;
};

function stringMap(value: unknown): Record<string, string> {
  if (!value || typeof value !== "object") return {};
  return Object.fromEntries(
    Object.entries(value as Record<string, unknown>).filter(
      (entry): entry is [string, string] => typeof entry[1] === "string",
    ),
  );
}

/** The request body the form submits: only the fields of its type. */
export function storageClassPayload(form: StorageClassFormState) {
  const base = {
    name: form.name.trim(),
    clusters: form.clusters,
    description: form.description,
  };
  if (form.type === "hostpath")
    return {
      ...base,
      type: "hostpath" as const,
      path: form.path.trim(),
      fs_type: form.fs_type.trim(),
    };
  if (form.type === "smb")
    return {
      ...base,
      type: "smb" as const,
      host: form.host,
      share: form.share,
      sub_path: form.sub_path,
      username: form.username,
      password: form.password,
      domain: form.domain,
    };
  const {
    path: _path,
    fs_type: _fsType,
    host: _host,
    share: _share,
    sub_path: _subPath,
    username: _username,
    password: _password,
    domain: _domain,
    ...s3
  } = form;
  return {
    ...s3,
    ...base,
    type: "s3" as const,
    mount_options: form.mount_options.map((line) => line.trim()).filter(Boolean),
    // Only clusters the storage has, and only those given an address.
    cluster_mount_endpoints: Object.fromEntries(
      Object.entries(form.cluster_mount_endpoints)
        .map(([cluster, endpoint]) => [cluster, endpoint.trim()])
        .filter(([cluster, endpoint]) => endpoint && form.clusters.includes(cluster)),
    ),
  };
}

export function normalizeStorageClass(raw: any): StorageClass {
  const name = raw?.name ?? raw?.id ?? "";
  return {
    id: raw?.id ?? name,
    name,
    namespace: raw?.namespace ?? "default",
    clusters: Array.isArray(raw?.clusters) ? raw.clusters : [],
    type: raw?.type ?? "s3",
    provider: (raw?.provider ?? "") as StorageProvider,
    endpoint: raw?.endpoint ?? "",
    mount_endpoint: raw?.mount_endpoint ?? "",
    cluster_mount_endpoints: stringMap(raw?.cluster_mount_endpoints),
    profiles: stringMap(raw?.profiles),
    default_profile: raw?.default_profile ?? "default",
    path: raw?.path ?? "",
    fs_type: raw?.fs_type ?? "",
    prefix: raw?.prefix ?? "",
    vfs_cache_mode: raw?.vfs_cache_mode === "off" ? "off" : "writes",
    mount_profile:
      raw?.mount_profile === "read" || raw?.mount_profile === "archive"
        ? raw.mount_profile
        : "default",
    mount_options: Array.isArray(raw?.mount_options)
      ? raw.mount_options.filter(
          (item: unknown): item is string => typeof item === "string",
        )
      : [],
    effective_mount_options: Array.isArray(raw?.effective_mount_options)
      ? raw.effective_mount_options
      : [],
    effective_backend_options: raw?.effective_backend_options ?? {},
    region: raw?.region ?? "",
    bucket: raw?.bucket ?? "",
    accessKeyId: raw?.accessKeyId ?? raw?.access_key_id ?? "",
    pathStyle: Boolean(raw?.pathStyle ?? raw?.path_style),
    host: raw?.host ?? "",
    share: raw?.share ?? "",
    subPath: raw?.subPath ?? "",
    username: raw?.username ?? "",
    domain: raw?.domain ?? "",
    description: raw?.description ?? "",
    createdAt: raw?.createdAt ?? raw?.created_at ?? "",
  };
}

export function storageClassToForm(
  storageClass?: StorageClass,
): StorageClassFormState {
  const accessKeyID = storageClass?.accessKeyId ?? "";
  return {
    name: storageClass?.name ?? "",
    clusters: storageClass?.clusters ?? [],
    type: storageClass?.type ?? "s3",
    provider: storageClass?.provider ?? "",
    endpoint: storageClass?.endpoint ?? "",
    mount_endpoint: storageClass?.mount_endpoint ?? "",
    cluster_mount_endpoints: { ...(storageClass?.cluster_mount_endpoints ?? {}) },
    prefix: storageClass?.prefix ?? "",
    vfs_cache_mode: storageClass?.vfs_cache_mode === "off" ? "off" : "writes",
    mount_profile: storageClass?.mount_profile ?? "default",
    mount_options: [...(storageClass?.mount_options ?? [])],
    region: storageClass?.region ?? "",
    bucket: storageClass?.bucket ?? "",
    access_key_id: accessKeyID,
    access_key_secret: "",
    path_style: storageClass?.pathStyle ?? false,
    host: storageClass?.host ?? "",
    share: storageClass?.share ?? "",
    sub_path: storageClass?.subPath ?? "",
    username: storageClass?.username ?? "",
    password: "",
    domain: storageClass?.domain ?? "",
    path: storageClass?.path ?? "",
    fs_type: storageClass?.fs_type ?? "",
    description: storageClass?.description ?? "",
  };
}

/** The volume a Job or Addon declares to mount the storage. */
export function storageVolumeExample(name: string, profile?: string): string {
  return [
    "volumes:",
    "  - name: data",
    "    csi:",
    "      driver: storage.rlark.io",
    "      volumeAttributes:",
    `        storage: ${name}`,
    ...(profile ? [`        profile: ${profile}`] : []),
    "containers:",
    "  - name: main",
    "    volumeMounts:",
    "      - name: data",
    "        mountPath: /mnt/data",
    "        subPath: my-dir",
  ].join("\n");
}
