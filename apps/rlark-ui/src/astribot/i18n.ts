import { copy, type Copy } from "../i18n";

// Our storage strings: the storage types, fields and hints this fork adds, and
// the page heading that names more than object storage.
const storageClass = {
  zh: {
    eyebrow: "存储",
    desc: "管理对象存储和 SMB/NAS 共享配置，用于任务数据持久化和工作负载存储。",
    mountEndpoint: "挂载 Endpoint（可选）",
    mountEndpointHint:
      "留空沿用 Endpoint。仅供目标集群 CSI 挂载使用；Gateway 校验和文件浏览仍使用上方 Endpoint。挂载地址连通性需在目标集群验证。",
    prefix: "对象前缀（可选）",
    prefixHint: "将 bucket 内该前缀作为挂载根目录。",
    vfsCacheMode: "写缓存",
    vfsCacheModeOff: "关闭",
    vfsCacheModeWrites: "启用（writes）",
    type: "存储类型",
    typeS3: "对象存储（S3 兼容）",
    typeSMB: "SMB / NAS 共享",
    typeHostPath: "节点目录（如 KPFS）",
    hostPath: "节点上的目录",
    hostPathHint:
      "节点已经挂好的文件系统（如金山云 KPFS 的 /mnt/xc_kpfs/regular）。集群装 Addon csi-driver-nodepath 后，按名字挂它的 Job 得到一个 PVC，驱动把节点上的目录 bind mount 进 Pod（不经 FUSE），Pod 只调度到驱动探测到有它的节点；挂不上时 Task 消息写明原因，节点上重挂后在 Pod 上记事件。",
    fsType: "文件系统类型",
    fsTypeHint:
      "目录所在文件系统的类型（/proc/mounts 里的写法，金山云 KPFS 是 kingstorage）。填了就必须相符；哪些节点有这个存储由驱动在每台节点上探测，不用打标签。",
    clusterEndpoints: "按集群的挂载端点",
    clusterEndpointsHint:
      "留空的集群用上面的挂载端点。云上集群填内网地址，机器人和办公室集群留空走公网。",
    renameHint: "改名后 Job 和 Addon 按新名字引用；StorageClass 和在跑的挂载都不变。",
    profiles: "挂载档位",
    usage: "在 Job / Addon 中引用",
    usageHint:
      "卷写存储名字，可选档位（profile）和只读（readOnly）；容器里用 volumeMounts 的 subPath 挂子目录。RLark 按 Task 所在集群把它换成对应的 StorageClass 或节点目录。",
    host: "服务器地址",
    share: "共享名",
    subPath: "子路径",
    username: "用户名",
    password: "密码",
    domain: "域",
    smbHint:
      "创建时只检查服务器 445 端口是否可达；账号密码在任务挂载时校验。SMB 存储类不支持在线浏览文件。",
  },
  en: {
    eyebrow: "Storage",
    desc: "Manage object storage and SMB/NAS share configurations for task data persistence and workload storage.",
    mountEndpoint: "Mount endpoint (optional)",
    mountEndpointHint:
      "Defaults to Endpoint. Used only by target-cluster CSI mounts; Gateway validation and browsing use Endpoint above. Validate mount connectivity in the target cluster.",
    prefix: "Object prefix (optional)",
    prefixHint: "Mount this prefix inside the bucket as the volume root.",
    vfsCacheMode: "VFS write cache",
    vfsCacheModeOff: "Off",
    vfsCacheModeWrites: "On (writes)",
    type: "Storage type",
    typeS3: "Object storage (S3-compatible)",
    typeSMB: "SMB / NAS share",
    typeHostPath: "Node directory (e.g. KPFS)",
    hostPath: "Directory on the nodes",
    hostPathHint:
      "A file system the nodes already mount (such as Kingsoft Cloud KPFS at /mnt/xc_kpfs/regular). With the Addon csi-driver-nodepath in the cluster, a Job naming it gets a claim whose driver bind-mounts the node's directory into the Pod (no FUSE), on the nodes the driver found to have it; a mount failure is in the Task's message, and a remount on the node is an event on the Pod.",
    fsType: "File system type",
    fsTypeHint:
      "The type of the file system the directory is on, as /proc/mounts lists it (Kingsoft Cloud KPFS is kingstorage). When given it must match. The driver finds on each node whether it has the storage: no node labels to set.",
    clusterEndpoints: "Mount endpoint per cluster",
    clusterEndpointsHint:
      "A cluster left empty uses the mount endpoint above. Give cloud clusters the internal address; robot and office clusters keep the public one.",
    renameHint: "After a rename, Jobs and Addons name the storage by its new name; its StorageClasses and running mounts do not change.",
    profiles: "Mount profiles",
    usage: "Use in a Job or Addon",
    usageHint:
      "A volume names the storage, optionally a profile and readOnly; containers mount a directory of it with volumeMounts subPath. RLark turns it into the StorageClass or node directory of the Task's cluster.",
    host: "Server",
    share: "Share",
    subPath: "Sub path",
    username: "Username",
    password: "Password",
    domain: "Domain",
    smbHint:
      "Creation only checks that port 445 on the server is reachable; credentials are verified when a task mounts the share. SMB storage classes do not support online file browsing.",
  },
};

// What our storage pages show: RLark's storage strings with ours.
export function storageCopy(c: Copy) {
  return { ...c.storageClass, ...storageClass[c === copy.zh ? "zh" : "en"] };
}
