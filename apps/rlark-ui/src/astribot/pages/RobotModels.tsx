import { Box, ChevronRight } from "lucide-react";
import { robotModelMocks } from "../lib/robotBody";
import "./RobotModels.css";

export function RobotModelsPage({ zh, navigate }: { zh: boolean; navigate: (id: string, sub?: string) => void }) {
  return <section className="table-page robot-models-page">
    <header className="page-header"><div><h1>{zh ? "机器人型号目录" : "Robot model catalog"}</h1><p>{zh ? "本体定义由型号目录维护。选择型号查看关节、末端与传感器规格。" : "Body definitions live in the model catalog. Select a model to inspect joints, end-effectors and sensors."}</p></div>
      <div className="page-actions"><button type="button" className="secondary-button" onClick={() => navigate("stations", "network")}>{zh ? "扫描设置" : "Discovery settings"}</button><button type="button" className="secondary-button" onClick={() => navigate("stations")}>{zh ? "返回站点" : "Back to stations"}</button></div>
    </header>
    <div className="robot-model-grid">
      {robotModelMocks.map((model) => <button type="button" className="robot-model-tile" key={model.name} onClick={() => navigate("stations", `models/${model.name}`)}>
        <span className="robot-model-tile-icon"><Box size={18} /></span>
        <span className="robot-model-tile-copy"><strong>{model.displayName}</strong><small>{model.vendor} · <code>{model.name}</code></small><span>{model.body ? (zh ? `${model.body.joints.length} 个关节 · ${model.body.endEffectors.length} 个末端` : `${model.body.joints.length} joints · ${model.body.endEffectors.length} end-effectors`) : (zh ? "尚未发布本体定义" : "Body definition not published")}</span></span>
        <ChevronRight size={17} aria-hidden="true" />
      </button>)}
    </div>
  </section>;
}
