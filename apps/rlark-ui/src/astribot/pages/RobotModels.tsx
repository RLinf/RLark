import { useEffect, useState } from "react";
import { Box, ChevronRight, RefreshCw, Trash2 } from "lucide-react";
import { robotModelsApi, type RobotModelItem } from "../api";
import "./RobotModels.css";

const referenceSummary = (model: RobotModelItem, zh: boolean) => {
  const r = model.references;
  const total = r.nodeClasses + r.nodes + r.machines + r.nodeAdmissions + r.nodeJoins;
  if (total === 0) return zh ? "无引用，可删除" : "No references, safe to delete";
  return zh
    ? `引用 · ${r.nodeClasses} 型号类 · ${r.nodes} 节点 · ${r.machines} 机器 · ${r.nodeAdmissions} 入网 · ${r.nodeJoins} 加入记录`
    : `References · ${r.nodeClasses} classes · ${r.nodes} nodes · ${r.machines} machines · ${r.nodeAdmissions} admissions · ${r.nodeJoins} joins`;
};

export function RobotModelsPage({ zh, navigate }: { zh: boolean; navigate: (id: string, sub?: string) => void }) {
  const [models, setModels] = useState<RobotModelItem[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [busy, setBusy] = useState("");

  const refresh = (signal?: AbortSignal) => {
    setLoading(true);
    robotModelsApi.list(signal).then((response) => {
      setModels(response.items);
      setError("");
      setLoading(false);
    }).catch((err: unknown) => {
      if (signal?.aborted) return;
      setError(err instanceof Error ? err.message : zh ? "加载失败" : "Load failed");
      setLoading(false);
    });
  };

  useEffect(() => {
    const controller = new AbortController();
    refresh(controller.signal);
    return () => controller.abort();
  }, []);

  const remove = (model: RobotModelItem) => {
    const r = model.references;
    if (r.nodeClasses + r.nodes + r.machines + r.nodeAdmissions + r.nodeJoins > 0) return;
    if (!window.confirm(zh ? `删除型号 ${model.name}？该操作不可撤销。` : `Delete model ${model.name}? This cannot be undone.`)) return;
    setBusy(model.name);
    robotModelsApi.delete(model.name).then(() => {
      setModels((current) => current.filter((entry) => entry.name !== model.name));
      setBusy("");
    }).catch((err: unknown) => {
      setError(err instanceof Error ? err.message : zh ? "删除失败" : "Delete failed");
      setBusy("");
    });
  };

  return <section className="table-page robot-models-page">
    <header className="page-header"><div><h1>{zh ? "机器人型号目录" : "Robot model catalog"}</h1><p>{zh ? "本体定义由型号目录维护。选择型号查看关节、末端与传感器规格。" : "Body definitions live in the model catalog. Select a model to inspect joints, end-effectors and sensors."}</p></div>
      <div className="page-actions"><button type="button" className="secondary-button" onClick={() => refresh()}><RefreshCw size={15} />{zh ? "刷新" : "Refresh"}</button><button type="button" className="secondary-button" onClick={() => navigate("stations", "network")}>{zh ? "扫描设置" : "Discovery settings"}</button><button type="button" className="secondary-button" onClick={() => navigate("stations")}>{zh ? "返回站点" : "Back to stations"}</button></div>
    </header>
    {error && <p className="error-message" role="alert">{error}</p>}
    {loading ? <p className="muted" role="status">{zh ? "加载中…" : "Loading…"}</p> : models.length === 0 ? <p className="muted">{zh ? "目录为空。" : "The catalog is empty."}</p> : <div className="robot-model-grid">
      {models.map((model) => {
        const guarded = model.references.nodeClasses + model.references.nodes + model.references.machines + model.references.nodeAdmissions + model.references.nodeJoins > 0;
        return <div className="robot-model-tile" key={model.name}>
          <button type="button" className="robot-model-open" onClick={() => navigate("stations", `models/${model.name}`)}>
            <span className="robot-model-tile-icon"><Box size={18} /></span>
            <span className="robot-model-tile-copy"><strong>{model.displayName || model.name}</strong><small>{model.vendor ? `${model.vendor} · ` : ""}<code>{model.name}</code></small><span>{referenceSummary(model, zh)}</span></span>
            <ChevronRight size={17} aria-hidden="true" />
          </button>
          <button type="button" className="robot-model-delete" title={guarded ? (zh ? "有引用的型号不能删除" : "Referenced models cannot be deleted") : (zh ? "删除" : "Delete")} disabled={guarded || busy === model.name} onClick={() => remove(model)}>{busy === model.name ? "…" : <Trash2 size={15} />}</button>
        </div>;
      })}
    </div>}
  </section>;
}
