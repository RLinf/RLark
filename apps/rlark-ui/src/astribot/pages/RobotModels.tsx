import { useEffect, useState } from "react";
import { Activity, Box, Camera, RefreshCw, Trash2 } from "lucide-react";
import { robotModelsApi, type RobotModelItem } from "../api";
import { partLabel } from "../lib/bodyCapabilities";
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
    <header className="page-header"><div><h1>{zh ? "机器人型号目录" : "Robot model catalog"}</h1><p>{zh ? "型号在这里只做展示与删除；新增由后台 apply 维护。入网流程与节点上报按这里的型号 ID 对齐。" : "Models are listed and deleted here; new models are applied on the backend. Join flows and node reports align on these model IDs."}</p></div>
      <div className="page-actions"><button type="button" className="secondary-button" onClick={() => refresh()}><RefreshCw size={15} />{zh ? "刷新" : "Refresh"}</button><button type="button" className="secondary-button" onClick={() => navigate("stations", "network")}>{zh ? "扫描设置" : "Discovery settings"}</button><button type="button" className="secondary-button" onClick={() => navigate("stations")}>{zh ? "返回站点" : "Back to stations"}</button></div>
    </header>
    {error && <p className="error-message" role="alert">{error}</p>}
    {loading ? <p className="muted" role="status">{zh ? "加载中…" : "Loading…"}</p> : models.length === 0 ? <p className="muted">{zh ? "目录为空。" : "The catalog is empty."}</p> : <div className="robot-model-grid">
      {models.map((model) => {
        const guarded = model.references.nodeClasses + model.references.nodes + model.references.machines + model.references.nodeAdmissions + model.references.nodeJoins > 0;
        return <article className="robot-model-card" key={model.name}>
          <header className="robot-model-header">
            <span className="robot-model-icon"><Box size={20} aria-hidden="true" /></span>
            <div className="robot-model-identity">
              <h2>{model.displayName || model.name}</h2>
              <small>{model.vendor ? `${model.vendor} · ` : ""}<code>{model.name}</code></small>
            </div>
            <span className="robot-model-references">{referenceSummary(model, zh)}</span>
            <button type="button" className="robot-model-delete" title={guarded ? (zh ? "有引用的型号不能删除" : "Referenced models cannot be deleted") : (zh ? "删除" : "Delete")} disabled={guarded || busy === model.name} onClick={() => remove(model)}>{busy === model.name ? "…" : <Trash2 size={15} />}</button>
          </header>
          {model.capabilities && <div className="robot-model-capabilities">
            <section>
              <h3><Camera size={16} aria-hidden="true" />{zh ? "相机" : "Cameras"}</h3>
              <dl className="robot-model-variants">{model.capabilities.variants.map((variant) => <div key={variant.id}>
                <dt><code>{variant.id}</code><small>{variant.cameras.length}{zh ? " 路彩色 JPEG" : " color JPEG views"}</small></dt>
                <dd>{variant.cameras.map((camera) => <span key={camera}>{partLabel(camera, zh)}</span>)}</dd>
              </div>)}</dl>
              <h4>{zh ? "末端与部件端点" : "End effectors and component endpoints"}</h4>
              <p>{model.capabilities.endpoints.map((point) => partLabel(point, zh)).join(zh ? "、" : ", ")}</p>
              <p>{zh ? "位置 3 维（XYZ） · 姿态 4 维（四元数 XYZW）" : "Position 3D (XYZ) · Orientation 4D (XYZW quaternion)"}</p>
            </section>
            <section>
              <h3><Activity size={16} aria-hidden="true" />{zh ? "本体关节" : "Body joints"}</h3>
              <table className="robot-model-joints">
                <thead><tr><th scope="col">{zh ? "部件" : "Component"}</th><th scope="col">{zh ? "维度" : "Dimensions"}</th><th scope="col">{zh ? "位置 / 速度" : "Position / velocity"}</th></tr></thead>
                <tbody>{model.capabilities.jointGroups.map((group) => <tr key={group.id}>
                  <th scope="row">{partLabel(group.id, zh)}</th><td>{group.dimensions}</td>
                  <td>{group.positionUnit && group.velocityUnit ? `${group.positionUnit} / ${group.velocityUnit}` : zh ? "驱动原始值" : "Raw driver values"}</td>
                </tr>)}</tbody>
              </table>
            </section>
          </div>}
        </article>;
      })}
    </div>}
  </section>;
}
