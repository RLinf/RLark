import { ArrowLeft, Battery, Camera, Cpu, Hand, Ruler, Weight } from "lucide-react";
import { compareRobotBody, robotModelMock, type RobotBodyComparison, type RobotBodyDifference } from "../lib/robotBody";
import "./RobotModelDetail.css";

const number = (value: number | undefined, suffix: string) => value === undefined ? "—" : `${value} ${suffix}`;
const stateLabel = (comparison: RobotBodyComparison, zh: boolean) => comparison.status === "matched"
  ? (zh ? "本体一致" : "Body matched")
  : comparison.status === "mismatch"
    ? (zh ? "本体不一致" : "Body mismatch")
    : (zh ? "待核验" : "Pending verification");

export function RobotModelDetailPage({ zh, modelID, navigate }: {
  zh: boolean;
  modelID: string;
  navigate: (id: string, sub?: string) => void;
}) {
  const model = robotModelMock(modelID);
  if (!model) return <section className="robot-body-page"><header className="page-header"><div><h1>{zh ? "型号不存在" : "Model not found"}</h1></div><button type="button" className="secondary-button" onClick={() => navigate("stations", "models")}><ArrowLeft size={15} />{zh ? "返回型号目录" : "Back to catalog"}</button></header></section>;
  const body = model.body;
  const matched = compareRobotBody(model, "s1u-01-x86");
  const mismatch = compareRobotBody(model, "s1u-02-x86");
  const pending = compareRobotBody(model, "s1u-03-x86");
  return <section className="robot-body-page">
    <header className="page-header robot-body-page-header">
      <div>
        <button type="button" className="robot-body-back" onClick={() => navigate("stations", "models")}><ArrowLeft size={15} />{zh ? "机器人型号目录" : "Robot model catalog"}</button>
        <h1>{model.displayName}</h1>
        <p>{model.vendor} · <code>{model.name}</code></p>
      </div>
      <span className="robot-body-readonly">{zh ? "型号目录 · 只读" : "Catalog · Read only"}</span>
    </header>

    {!body ? <section className="robot-body-undefined" role="status"><Cpu size={20} /><div><strong>{zh ? "尚未发布本体定义" : "Body definition not published"}</strong><p>{model.description}</p><small>{zh ? "当前型号目录项仍可使用；设备上报可独立查看。" : "This catalog entry remains usable. Device reports can still be viewed independently."}</small></div></section> : <>
      <div className="robot-body-facts">
        <section className="robot-body-section robot-body-joints">
          <header><Cpu size={17} /><div><h2>{zh ? "关节" : "Joints"}</h2><small>{body.joints.length} {zh ? "项 · 型号定义" : "defined joints"}</small></div></header>
          {body.joints.length ? <div className="robot-body-table-scroll"><table className="robot-body-table"><thead><tr><th>{zh ? "名称" : "Name"}</th><th>{zh ? "类型" : "Type"}</th><th>{zh ? "单位" : "Units"}</th><th>{zh ? "限位" : "Limits"}</th></tr></thead><tbody>
            {body.joints.map((joint) => <tr key={joint.name}><th scope="row"><code>{joint.name}</code></th><td>{joint.type}</td><td>{joint.units}</td><td>{joint.limits ? `${joint.limits.lower} — ${joint.limits.upper}` : (zh ? "未公布" : "Not published")}</td></tr>)}
          </tbody></table></div> : <p className="robot-body-empty">{zh ? "型号未声明关节。" : "No joints declared."}</p>}
        </section>

        <div className="robot-body-side">
          <section className="robot-body-section">
            <header><Hand size={17} /><div><h2>{zh ? "末端执行器" : "End-effectors"}</h2><small>{zh ? "安装装位与自由度" : "Mounts and degrees of freedom"}</small></div></header>
            <div className="robot-body-item-list">{body.endEffectors.map((item) => <div className="robot-body-item" key={item.name}><strong>{item.name}</strong><span>{item.kind} · {item.dof} DOF</span>{item.placement && <small>{zh ? "装位" : "Mount"} · {item.placement}</small>}</div>)}</div>
          </section>
          <section className="robot-body-section">
            <header><Camera size={17} /><div><h2>{zh ? "传感器" : "Sensors"}</h2><small>{zh ? "型号配置" : "Model specification"}</small></div></header>
            <div className="robot-body-item-list">{body.sensors.map((item) => <div className="robot-body-item" key={item.name}><strong>{item.name}</strong><span>{item.kind} × {item.count}</span><small>{zh ? "装位" : "Mount"} · {item.placement}</small></div>)}</div>
          </section>
          <section className="robot-body-section">
            <header><Ruler size={17} /><div><h2>{zh ? "本体参数" : "Physical specifications"}</h2><small>{zh ? "型号额定值" : "Model ratings"}</small></div></header>
            <div className="robot-body-specs">
              <div><Weight size={15} /><span>{zh ? "额定负载" : "Payload"}</span><strong>{number(body.physicalSpecs?.payloadKg, "kg")}</strong></div>
              <div><Battery size={15} /><span>{zh ? "标称续航" : "Runtime"}</span><strong>{number(body.physicalSpecs?.runtimeMinutes, zh ? "分钟" : "min")}</strong></div>
              <div><Ruler size={15} /><span>{zh ? "尺寸" : "Dimensions"}</span><strong>{body.physicalSpecs?.dimensionsMm ? `${body.physicalSpecs.dimensionsMm.length} × ${body.physicalSpecs.dimensionsMm.width} × ${body.physicalSpecs.dimensionsMm.height} mm` : "—"}</strong></div>
              <div><Battery size={15} /><span>{zh ? "电池" : "Battery"}</span><strong>{body.physicalSpecs?.battery ? [number(body.physicalSpecs.battery.capacityWh, "Wh"), number(body.physicalSpecs.battery.nominalVoltageV, "V")].filter((item) => item !== "—").join(" · ") || "—" : "—"}</strong></div>
            </div>
          </section>
        </div>
      </div>

      <section className="robot-body-comparison-section">
        <header className="robot-body-comparison-header"><div><h2>{zh ? "站点机器人实例对照" : "Robot instance comparison"}</h2><p>{zh ? "当前设备枚举与型号目录对照；此状态不控制就绪或调度。" : "Device inventory compared with the catalog. This status does not control readiness or scheduling."}</p></div><span className="robot-body-source">{zh ? "只读 mock 样例" : "Read-only mock fixtures"}</span></header>
        <div className="robot-body-node-grid">
          <ComparisonCard name="s1u-01-x86" comparison={matched} zh={zh} />
          <ComparisonCard name="s1u-02-x86" comparison={mismatch} zh={zh} />
          <ComparisonCard name="s1u-03-x86" comparison={pending} zh={zh} />
        </div>
      </section>
    </>}
  </section>;
}

function ComparisonCard({ name, comparison, zh }: { name: string; comparison: RobotBodyComparison; zh: boolean }) {
  const observed = comparison.observed;
  return <article className={"robot-body-node-card is-" + comparison.status}>
    <header><strong>{name}</strong><span className={"robot-body-status is-" + comparison.status}><i />{stateLabel(comparison, zh)}</span></header>
    <p className="robot-body-observation-note">{observed ? `${observed.source === "driverInventory" ? (zh ? "驱动枚举" : "Driver inventory") : (zh ? "运行时描述" : "Runtime description")} · ${observed.state === "complete" ? (zh ? "完整清单" : "Complete inventory") : (zh ? "部分清单" : "Partial inventory")} · ${zh ? `采样 ${observed.sampleAgeSeconds ?? "—"} 秒前` : `sampled ${observed.sampleAgeSeconds ?? "—"} s ago`}` : (zh ? "没有实例清单" : "No instance inventory")}</p>
    {comparison.differences.length > 0 ? <ul>{comparison.differences.map((item) => <DifferenceRow key={item.section + item.name} item={item} zh={zh} />)}</ul> : <p className="robot-body-matched-copy">{zh ? "目录定义与完整设备清单一致。" : "The catalog matches the complete device inventory."}</p>}
  </article>;
}

function DifferenceRow({ item, zh }: { item: RobotBodyDifference; zh: boolean }) {
  const labels = {
    missing: zh ? "缺失" : "Missing",
    extra: zh ? "额外" : "Extra",
    conflict: zh ? "冲突" : "Conflict",
    evidence: zh ? "证据不足" : "Insufficient evidence",
  };
  return <li className={"robot-body-difference is-" + item.kind}><span>{labels[item.kind]}</span><code>{item.name}</code><small>{item.expected ?? "—"} <b>→</b> {item.observed ?? "—"}</small></li>;
}
