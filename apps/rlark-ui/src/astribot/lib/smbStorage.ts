import type { StorageClass } from "../../data";

/** 列表和详情中“提供商”一栏：SMB 类型统一显示 SMB。 */
export function storageProviderLabel(
  storageClass: Pick<StorageClass, "type" | "provider">,
): string {
  if (storageClass.type === "hostpath") return "Host path";
  return storageClass.type === "smb" ? "SMB" : storageClass.provider;
}

/** 列表和详情中“位置”一栏：S3 显示存储桶，SMB 显示共享路径。 */
export function storageLocation(
  storageClass: Pick<StorageClass, "type" | "bucket" | "host" | "share" | "subPath" | "path" | "prefix">,
): string {
  if (storageClass.type === "hostpath") return storageClass.path ?? "";
  if (storageClass.type === "smb") return smbLocation(storageClass);
  return [storageClass.bucket, storageClass.prefix].filter(Boolean).join("/");
}

/** SMB 存储类展示为 //host/share/subPath。 */
export function smbLocation(
  storageClass: Pick<StorageClass, "host" | "share" | "subPath">,
): string {
  const path = [storageClass.share, storageClass.subPath].filter(Boolean).join("/");
  return `//${storageClass.host}${path ? `/${path}` : ""}`;
}
