import type { CRDVolume } from "../../crdModels";

/** The CSI driver name with which a volume names an RLark storage. */
export const STORAGE_DRIVER = "storage.rlark.io";

type StorageMount = { objectStorage: string; profile: string; readOnly: boolean };

// A storage mount names the storage; RLark resolves it to the StorageClass or
// node directory of the cluster the Task runs in.
export function storageVolume(name: string, mount: StorageMount) {
  return {
    name,
    csi: {
      driver: STORAGE_DRIVER,
      ...(mount.readOnly ? { readOnly: true } : {}),
      volumeAttributes: {
        storage: mount.objectStorage,
        ...(mount.profile ? { profile: mount.profile } : {}),
      },
    },
  };
}

// The form's mount for a volume that names a storage.
export function storageMount(csi: NonNullable<CRDVolume["csi"]>, mountPath: string) {
  return {
    type: "storage" as const,
    objectStorage: csi.volumeAttributes?.storage ?? "",
    mountPath,
    hostPath: "",
    profile: csi.volumeAttributes?.profile ?? "",
    readOnly: csi.readOnly ?? false,
  };
}

// A storage mount's profile and access, as a Job's settings list them.
export function storageMountLabel(mount: { profile: string; readOnly: boolean }, zh: boolean) {
  return [mount.profile || (zh ? "默认" : "default"), mount.readOnly ? (zh ? "只读" : "read-only") : ""].filter(Boolean).join(" · ");
}
