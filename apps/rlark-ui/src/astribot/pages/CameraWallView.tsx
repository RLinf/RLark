import { useLayoutEffect, useRef, useState } from "react";
import { cameraMediaScheduler } from "../lib/cameraMediaScheduler";
import { cameraCells, DEFAULT_CAMERA_FPS, publishedCameras, type CameraFps, type CameraView } from "../lib/cameras";
import type { FleetStation, StationFleetMeta } from "../lib/stationFleet";
import { stationPhase, stationPhaseLabel, stationStatusClass } from "../lib/stationDisplay";
import { CameraWall } from "./StationCard";
import { fleetAlertLabel, fleetKey } from "./StationsTableView";

export function FleetPagination({ page, pageCount, zh, onPage }: {
  page: number; pageCount: number; zh: boolean; onPage: (page: number) => void;
}) {
  return <div className="stations-pagination">
    <button type="button" className="secondary-button" disabled={page <= 1} onClick={() => onPage(page - 1)}>{zh ? "上一页" : "Previous"}</button>
    <span aria-live="polite">{page} / {pageCount}</span>
    <button type="button" className="secondary-button" disabled={page >= pageCount} onClick={() => onPage(page + 1)}>{zh ? "下一页" : "Next"}</button>
  </div>;
}

function WallTile({ item, meta, zh, onPreview }: {
  item: FleetStation;
  meta: StationFleetMeta;
  zh: boolean;
  onPreview: (item: FleetStation, view: CameraView, trigger: HTMLElement, fps?: CameraFps) => void;
}) {
  const measuredFps = useRef<HTMLSpanElement>(null);
  const phase = stationPhase(item);
  const cells = cameraCells(publishedCameras(item.site?.cameras ?? item.annotations?.["rlark.io/cameras"]), zh);
  const alert = fleetAlertLabel(meta, zh);
  return <article className="stations-wall-tile" data-severity={meta.alertSeverity}>
    <CameraWall cells={cells} node={item.metadata.name} namespace={item.metadata.namespace} zh={zh} online={phase === "Bound"}
      fps={DEFAULT_CAMERA_FPS} measuredFps={measuredFps} onZoom={(view, trigger, fps) => onPreview(item, view, trigger, fps)} monitor />
    <footer>
      <strong title={item.metadata.name}>{item.metadata.name}</strong>
      <span className={`status ${stationStatusClass(phase)}`}><i />{stationPhaseLabel(phase, zh)}</span>
      {alert && <span className="stations-table-alert">{alert}</span>}
    </footer>
  </article>;
}

export function CameraWallView({ items, metaByKey, zh, onPreview, onClear, selected }: {
  items: FleetStation[];
  metaByKey: ReadonlyMap<string, StationFleetMeta>;
  zh: boolean;
  selected: boolean;
  onClear: () => void;
  onPreview: (item: FleetStation, view: CameraView, trigger: HTMLElement, fps?: CameraFps) => void;
}) {
  const [columns, setColumns] = useState<2 | 3>(2);
  const [page, setPage] = useState(1);
  const size = columns * columns;
  const pageCount = Math.max(1, Math.ceil(items.length / size));
  const currentPage = Math.min(page, pageCount);
  useLayoutEffect(() => {
    cameraMediaScheduler.setMaxLiveStreams(size);
    return () => cameraMediaScheduler.setMaxLiveStreams(3);
  }, [size]);

  return <section className="stations-wall" aria-label={zh ? "监控墙" : "Camera wall"}>
    <div className="stations-view-options">
      <div className="stations-segments" role="group" aria-label={zh ? "宫格大小" : "Grid size"}>
        {([2, 3] as const).map((value) => <button key={value} type="button" aria-pressed={columns === value} onClick={() => { setColumns(value); setPage(1); }}>{value} × {value}</button>)}
      </div>
      {columns === 3 && <small className="muted">{zh ? "HTTP/1.1 浏览器连接上限可能让部分画面排队。" : "The browser’s HTTP/1.1 connection limit may queue some streams."}</small>}
      {selected && <button type="button" className="secondary-button" onClick={() => { onClear(); setPage(1); }}>{zh ? "显示全部机器人" : "Show all robots"}</button>}
      <FleetPagination page={currentPage} pageCount={pageCount} zh={zh} onPage={setPage} />
    </div>
    {items.length ? <div className={`stations-wall-grid is-${columns}`}>
      {items.slice((currentPage - 1) * size, currentPage * size).map((item) => {
        const key = fleetKey(item);
        return <WallTile key={key} item={item} meta={metaByKey.get(key)!} zh={zh} onPreview={onPreview} />;
      })}
    </div> : <div className="stations-state muted">{zh ? "没有匹配的机器人" : "No matching robots"}</div>}
  </section>;
}
