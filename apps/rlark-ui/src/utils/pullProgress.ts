// node-agent 以固定英文文案上报镜像拉取进度（见
// apps/rlark/pkg/agent/imagepull/runtime.go），后端不做本地化。前端渲染
// PullProgressEntry.message 时需要逐条映射中文，否则中文界面会直接显示英文。
// 未识别的文案原样返回，避免丢失排查信息。

const PULL_MESSAGE_LABELS: Record<string, string> = {
  "image already present": "镜像已存在",
  "waiting for pull to start": "等待开始拉取",
  "waiting for image to be pulled": "等待镜像拉取完成",
  "extracting layers": "解压镜像层",
  "image pulled successfully": "镜像拉取完成",
};

const PULL_STATUS_LABELS: Record<string, string> = {
  pulling: "拉取中",
  completed: "已完成",
  failed: "失败",
};

const DOWNLOADING_PATTERN = /^downloading: (.+) \/ (.+) \((\d+) blobs\)$/;
const DOWNLOADING_TOTAL_PATTERN = /^downloading: ([^/]+?) \((\d+) blobs\)$/;
const WAITING_PULL_PATTERN = /^waiting for pull to start \((\d+)s elapsed\)$/;
const WAITING_IMAGE_PATTERN =
  /^waiting for image to be pulled \((\d+)s elapsed\)$/;

// localizePullMessage 把后端上报的英文 message 翻译为中文；无法识别时返回
// undefined，由调用方决定回退到原始 message。
function localizePullMessage(message: string): string | undefined {
  const known = PULL_MESSAGE_LABELS[message];
  if (known) return known;

  const downloading = DOWNLOADING_PATTERN.exec(message);
  if (downloading) {
    const [, downloaded, total, blobs] = downloading;
    return `下载中：${downloaded} / ${total}（${blobs} 个 blob）`;
  }
  const downloadingTotal = DOWNLOADING_TOTAL_PATTERN.exec(message);
  if (downloadingTotal) {
    const [, downloaded, blobs] = downloadingTotal;
    return `下载中：${downloaded}（${blobs} 个 blob）`;
  }
  const waitingPull = WAITING_PULL_PATTERN.exec(message);
  if (waitingPull) return `等待开始拉取（已等待 ${waitingPull[1]} 秒）`;

  const waitingImage = WAITING_IMAGE_PATTERN.exec(message);
  if (waitingImage) return `等待镜像拉取完成（已等待 ${waitingImage[1]} 秒）`;

  return undefined;
}

// pullProgressLabel 返回进度条目展示文案：优先使用 message，缺失时回退到
// status。英文界面保持后端原值，中文界面翻译已知文案。
export function pullProgressLabel(
  message: string | undefined,
  status: string | undefined,
  zh: boolean,
): string {
  const raw = message || status || "";
  if (!zh || !raw) return raw;
  if (message) return localizePullMessage(message) ?? message;
  return PULL_STATUS_LABELS[status ?? ""] ?? raw;
}
