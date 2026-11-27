import { useState } from "react";
import { ChevronUp, X } from "lucide-react";
import type { RobotPower } from "./useRobotPower";

export function BatchProgressPanel({
  batch,
  cancelBatch,
  retryFailed,
  dismissBatch,
  zh,
}: Pick<
  RobotPower,
  "batch" | "cancelBatch" | "retryFailed" | "dismissBatch"
> & { zh: boolean }) {
  const [expanded, setExpanded] = useState(false);
  if (!batch) return null;
  const { succeeded, running, queued, failed, cancelled, total } = batch;
  const canDismiss = running === 0 && queued === 0;
  return (
    <aside
      className="stations-batch-panel"
      aria-label={zh ? "批量操作进度" : "Batch operation progress"}
    >
      <div className="stations-batch-panel-header">
        <button
          type="button"
          className="stations-batch-expand"
          aria-expanded={expanded}
          aria-controls="stations-batch-items"
          onClick={() => setExpanded(!expanded)}
        >
          <strong>{batch.action}</strong>
          <ChevronUp size={16} aria-hidden="true" />
        </button>
        {canDismiss && (
          <button
            type="button"
            className="stations-batch-close"
            onClick={dismissBatch}
            aria-label={zh ? "关闭进度" : "Dismiss progress"}
          >
            <X size={16} />
          </button>
        )}
      </div>
      <progress
        value={succeeded + failed + cancelled}
        max={total}
        aria-label={zh ? "已处理台数" : "Robots processed"}
      />
      <div className="stations-batch-counts" aria-live="polite">
        <span>
          {zh ? "成功" : "Succeeded"} {succeeded}
        </span>
        <span>
          {zh ? "进行中" : "Running"} {running}
        </span>
        <span>
          {zh ? "排队" : "Queued"} {queued}
        </span>
        <span className={failed ? "stations-table-alert" : ""}>
          {zh ? "失败" : "Failed"} {failed}
        </span>
        {cancelled > 0 && (
          <span>
            {zh ? "已取消" : "Cancelled"} {cancelled}
          </span>
        )}
      </div>
      {(queued > 0 || failed > 0) && (
        <div className="stations-batch-panel-actions">
          {queued > 0 && (
            <button
              type="button"
              className="secondary-button"
              onClick={cancelBatch}
            >
              {zh ? "取消排队" : "Cancel queued"}
            </button>
          )}
          {failed > 0 && (
            <button
              type="button"
              className="secondary-button"
              disabled={!canDismiss}
              onClick={() => void retryFailed()}
            >
              {zh ? `仅重试失败的 ${failed} 台` : `Retry only ${failed} failed`}
            </button>
          )}
        </div>
      )}
      {expanded && (
        <ul id="stations-batch-items" className="stations-batch-items">
          {batch.items
            .filter((item) => item.status === "failed")
            .map((item) => (
              <li key={item.key}>
                <strong>{item.node}</strong>
                <span>{item.error}</span>
              </li>
            ))}
          {failed === 0 && <li>{zh ? "暂无失败记录" : "No failures"}</li>}
        </ul>
      )}
    </aside>
  );
}
