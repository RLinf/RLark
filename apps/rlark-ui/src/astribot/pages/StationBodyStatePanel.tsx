import { useEffect, useId, useState } from "react";
import { Activity } from "lucide-react";
import { stationsApi } from "../api";
import { partLabel, type StationBodyState } from "../lib/bodyCapabilities";
import "./StationBodyStatePanel.css";

function SampleStatus({ state, age, zh }: { state?: string; age?: number; zh: boolean }) {
  const label = state === "live" ? (zh ? "实时" : "Live")
    : state === "stale" ? (zh ? "已过期" : "Stale") : (zh ? "待上报" : "Waiting");
  const detail = age === undefined ? undefined : zh ? `${age.toFixed(1)} 秒前收到` : `Received ${age.toFixed(1)} s ago`;
  return <span className="station-body-state" data-state={state} title={detail}>{label}</span>;
}

const value = (number?: number) => number === undefined ? "—" : (Math.abs(number) < 0.0005 ? 0 : number).toFixed(3);
const positionAxes = ["X", "Y", "Z"];
const orientationAxes = [...positionAxes, "W"];

function PoseValues({ axes, values }: { axes: string[]; values?: number[] }) {
  return <span className="station-body-vector">{axes.map((axis, i) => <span className="station-body-axis" key={axis}>
    <small>{axis}</small><span>{value(values?.[i])}</span>
  </span>)}</span>;
}

export function StationBodyStatePanel({ name, clusterID, uid, zh }: { name: string; clusterID: string; uid: string; zh: boolean }) {
  const headingId = useId();
  const [data, setData] = useState<StationBodyState>();
  const [failed, setFailed] = useState(false);
  useEffect(() => {
    setData(undefined);
    setFailed(false);
    const controller = new AbortController();
    let timer: ReturnType<typeof setTimeout>;
    let busy = false;
    const poll = async () => {
      if (busy || document.hidden || controller.signal.aborted) return;
      busy = true;
      try {
        const result = await stationsApi.bodyState(name, clusterID, uid, controller.signal);
        if (controller.signal.aborted) return;
        setData(result);
        setFailed(false);
      } catch {
        if (controller.signal.aborted) return;
        setData(undefined);
        setFailed(true);
      } finally {
        busy = false;
        if (!controller.signal.aborted && !document.hidden) timer = setTimeout(poll, 1000);
      }
    };
    const visibility = () => {
      clearTimeout(timer);
      if (!document.hidden) void poll();
    };
    document.addEventListener("visibilitychange", visibility);
    void poll();
    return () => {
      document.removeEventListener("visibilitychange", visibility);
      clearTimeout(timer);
      controller.abort();
    };
  }, [name, clusterID, uid]);

  const profile = data?.capabilities;
  const jointGroups = (profile?.jointGroups ?? []).map((group) => ({
    ...group, sample: data?.snapshot?.joints.find((item) => item.id === group.id),
  }));
  const jointRows = Math.max(0, ...jointGroups.map((group) => group.dimensions));
  const endpointLabel = zh ? "末端位姿" : "Endpoint poses";
  const jointLabel = zh ? "关节位置与速度" : "Joint position and velocity";
  const variant = profile?.variants.find((item) => item.id === data?.body);
  return <section className="station-body" aria-labelledby={headingId}>
    <header className="station-body-heading">
      <h4 id={headingId}><Activity size={16} aria-hidden="true" />{zh ? "本体状态" : "Body state"}</h4>
      {variant && <p><code>{variant.id}</code><span>{zh ? "配置相机" : "Configured cameras"} · {variant.cameras.map((camera) => partLabel(camera, zh)).join(" / ")}</span></p>}
    </header>
    <div className="station-body-content">
      {data?.state !== "available" ? <p className="muted" role="status">{failed
        ? (zh ? "暂时无法读取本体状态" : "Body state is temporarily unavailable")
        : data?.state === "unsupported" ? (zh ? "该型号或硬件变体暂无本体观测配置" : "Body observation is unavailable for this model or variant")
        : data?.state === "unavailable" ? (zh ? "本体采集器未就绪" : "Body observer is not ready")
        : (zh ? "正在读取本体状态…" : "Reading body state…")}</p> : <>
        <h5 id={`${headingId}-poses`}>{endpointLabel}</h5>
        <div className="station-body-table-wrap" role="region" aria-labelledby={`${headingId}-poses`} tabIndex={0}>
          <table className="station-body-table station-body-poses" aria-labelledby={`${headingId}-poses`}>
            <colgroup>
              <col className="station-body-label-col" /><col className="station-body-position-col" /><col />
              <col className="station-body-frame-col" /><col className="station-body-status-col" />
            </colgroup>
            <thead>
              <tr>
                <th scope="col">{zh ? "部件" : "Component"}</th>
                <th scope="col">{zh ? "位置" : "Position"}</th>
                <th scope="col">{zh ? "四元数" : "Quaternion"}</th>
                <th scope="col">{zh ? "坐标系" : "Frame"}</th>
                <th scope="col">{zh ? "状态" : "Status"}</th>
              </tr>
            </thead>
            <tbody>{profile?.endpoints.map((id) => {
              const sample = data.snapshot?.endpoints.find((item) => item.id === id);
              const live = sample?.state === "live";
              return <tr key={id}>
                <th scope="row">{partLabel(id, zh)}</th>
                <td><PoseValues axes={positionAxes} values={live ? sample.position : undefined} /></td>
                <td><PoseValues axes={orientationAxes} values={live ? sample.quaternion : undefined} /></td>
                <td className="station-body-frame">{live ? sample.frame || (zh ? "未提供" : "Not provided") : "—"}</td>
                <td><SampleStatus state={sample?.state} age={sample?.ageSeconds} zh={zh} /></td>
              </tr>;
            })}</tbody>
          </table>
        </div>
        <h5 id={`${headingId}-joints`}>{jointLabel}</h5>
        <div className="station-body-table-wrap" role="region" aria-labelledby={`${headingId}-joints`} tabIndex={0}>
          <table className="station-body-table station-body-joints" aria-labelledby={`${headingId}-joints`}>
            <colgroup><col className="station-body-label-col" /></colgroup>
            {jointGroups.map((group) => <colgroup key={group.id} span={2} />)}
            <thead>
              <tr>
                <th scope="col" rowSpan={2}>{zh ? "编号" : "Joint no."}</th>
                {jointGroups.map((group) => <th scope="colgroup" colSpan={2} key={group.id}>
                  <span className="station-body-joint-group">
                    <span className="station-body-group-label">{partLabel(group.id, zh)}</span>
                    <span className="station-body-dimension">{group.dimensions}{zh ? " 维" : " values"}</span>
                    <SampleStatus state={group.sample?.state} age={group.sample?.ageSeconds} zh={zh} />
                  </span>
                </th>)}
              </tr>
              <tr>{jointGroups.flatMap((group) => [
                <th className="station-body-group-start" scope="col" key={`${group.id}-p`}>{zh ? "位置" : "Position"} <small>{group.positionUnit ?? (zh ? "原始值" : "raw")}</small></th>,
                <th scope="col" key={`${group.id}-v`}>{zh ? "速度" : "Velocity"} <small>{group.velocityUnit ?? (zh ? "原始值" : "raw")}</small></th>,
              ])}</tr>
            </thead>
            <tbody>{Array.from({ length: jointRows }, (_, i) => <tr key={i}>
              <th scope="row">{i + 1}</th>
              {jointGroups.flatMap((group) => {
                const missing = i >= group.dimensions;
                const sample = !missing && group.sample?.state === "live" ? group.sample : undefined;
                const missingLabel = missing ? (zh ? "不适用" : "Not applicable") : undefined;
                return [
                  <td className="station-body-group-start" data-missing={missing} aria-label={missingLabel} key={`${group.id}-p`}>{missing ? null : value(sample?.position?.[i])}</td>,
                  <td data-missing={missing} aria-label={missingLabel} key={`${group.id}-v`}>{missing ? null : value(sample?.velocity?.[i])}</td>,
                ];
              })}
            </tr>)}</tbody>
          </table>
        </div>
      </>}
    </div>
  </section>;
}
