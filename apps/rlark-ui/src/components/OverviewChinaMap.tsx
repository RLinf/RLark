import { useEffect, useMemo, useRef, useState } from "react";
import {
  Bot,
  CloudCog,
  MapPin,
  Minus,
  Network,
  Plus,
  RotateCcw,
  Server,
  Workflow,
} from "lucide-react";
import type { Copy } from "../i18n";
import type { CRDNode, Page } from "../types";
import type { Cluster, Job } from "../data";
import { getNodeCategories, getNodeLocation } from "../utils/nodes";

type Position = [number, number];
type Geometry = {
  type: "Polygon" | "MultiPolygon";
  coordinates: Position[][] | Position[][][];
};
type Feature = {
  properties?: { name?: string; adcode?: number | string };
  geometry: Geometry;
};
type FeatureCollection = { features: Feature[] };

const VIEWBOX = {
  width: 800,
  height: 520,
  minLon: 73,
  maxLon: 135,
  minLat: 18,
  maxLat: 54,
};

const cityCoords: Record<string, [number, number]> = {
  北京市: [116.41, 39.9],
  上海市: [121.47, 31.23],
  杭州市: [120.15, 30.27],
  深圳市: [114.06, 22.54],
  广州市: [113.26, 23.13],
  成都市: [104.06, 30.67],
  重庆市: [106.55, 29.56],
  武汉市: [114.31, 30.59],
  西安市: [108.94, 34.34],
  南京市: [118.8, 32.06],
  青岛市: [120.38, 36.07],
  长沙市: [112.94, 28.23],
  天津市: [117.2, 39.13],
  苏州市: [120.58, 31.3],
  郑州市: [113.62, 34.75],
  合肥市: [117.27, 31.86],
  福州市: [119.3, 26.08],
  厦门市: [118.09, 24.48],
  南昌市: [115.86, 28.68],
  济南市: [117.0, 36.65],
  沈阳市: [123.43, 41.8],
  长春市: [125.32, 43.82],
  哈尔滨市: [126.64, 45.75],
  大连市: [121.61, 38.91],
  石家庄市: [114.5, 38.05],
  太原市: [112.55, 37.87],
  兰州市: [103.84, 36.06],
  银川市: [106.23, 38.49],
  西宁市: [101.78, 36.62],
  乌鲁木齐市: [87.62, 43.82],
  拉萨市: [91.13, 29.65],
  呼和浩特市: [111.75, 40.84],
  贵阳市: [106.71, 26.57],
  昆明市: [102.83, 24.88],
  南宁市: [108.37, 22.82],
  海口市: [110.32, 20.03],
  宁波市: [121.55, 29.87],
  无锡市: [120.3, 31.57],
  佛山市: [113.12, 23.02],
  东莞市: [113.75, 23.05],
  珠海市: [113.58, 22.27],
  中山市: [113.39, 22.52],
  惠州市: [114.41, 23.11],
};

interface CityData {
  name: string;
  lon: number;
  lat: number;
  nodes: number;
  clusters: number;
  cloud: number;
  edge: number;
  robot: number;
  dominant: "cloud" | "edge" | "robot";
}

function parseNodeLocations(nodes: CRDNode[]): {
  cities: CityData[];
  totalByCat: { cloud: number; edge: number; robot: number };
} {
  const cityMap = new Map<
    string,
    {
      nodes: number;
      clusters: Set<string>;
      cloud: number;
      edge: number;
      robot: number;
    }
  >();
  const totalByCat = { cloud: 0, edge: 0, robot: 0 };

  for (const node of nodes) {
    const city = getNodeLocation(node);
    if (!city) continue;

    const categories = getNodeCategories(node).filter(
      (category) => category !== "unknown",
    );
    categories.forEach((category) => totalByCat[category]++);

    const ns = node.metadata.namespace ?? "";
    if (!cityMap.has(city)) {
      cityMap.set(city, {
        nodes: 0,
        clusters: new Set(),
        cloud: 0,
        edge: 0,
        robot: 0,
      });
    }
    const entry = cityMap.get(city)!;
    entry.nodes++;
    if (ns) entry.clusters.add(ns);
    categories.forEach((category) => entry[category]++);
  }

  const cities: CityData[] = [];
  for (const [name, entry] of cityMap) {
    const coords = cityCoords[name];
    if (!coords) continue;
    const dominant = (
      [
        ["cloud", entry.cloud],
        ["edge", entry.edge],
        ["robot", entry.robot],
      ] as Array<["cloud" | "edge" | "robot", number]>
    ).sort((a, b) => b[1] - a[1])[0][0];
    cities.push({
      name,
      lon: coords[0],
      lat: coords[1],
      nodes: entry.nodes,
      clusters: entry.clusters.size,
      cloud: entry.cloud,
      edge: entry.edge,
      robot: entry.robot,
      dominant,
    });
  }

  cities.sort((a, b) => b.nodes - a.nodes);
  return { cities, totalByCat };
}

function buildConnections(cities: CityData[]): [number, number][] {
  if (cities.length < 2) return [];
  const conns: [number, number][] = [];
  for (let i = 1; i < cities.length; i++) {
    conns.push([0, i]);
  }
  for (let i = 1; i < cities.length && i <= 4; i++) {
    for (let j = i + 1; j < cities.length && j <= 4; j++) {
      conns.push([i, j]);
    }
  }
  return conns;
}

function project(lon: number, lat: number) {
  return {
    x:
      ((lon - VIEWBOX.minLon) / (VIEWBOX.maxLon - VIEWBOX.minLon)) *
      VIEWBOX.width,
    y:
      ((VIEWBOX.maxLat - lat) / (VIEWBOX.maxLat - VIEWBOX.minLat)) *
      VIEWBOX.height,
  };
}

function ringPath(ring: Position[]) {
  return (
    ring
      .map(([lon, lat], index) => {
        const { x, y } = project(lon, lat);
        return `${index === 0 ? "M" : "L"}${x.toFixed(1)},${y.toFixed(1)}`;
      })
      .join(" ") + " Z"
  );
}

function geometryPath(geometry: Geometry) {
  const polygons =
    geometry.type === "Polygon"
      ? [geometry.coordinates as Position[][]]
      : (geometry.coordinates as Position[][][]);
  return polygons.flatMap((polygon) => polygon.map(ringPath)).join(" ");
}

function connectionPath(
  cities: CityData[],
  fromIndex: number,
  toIndex: number,
) {
  const fromCity = cities[fromIndex];
  const toCity = cities[toIndex];
  const from = project(fromCity.lon, fromCity.lat);
  const to = project(toCity.lon, toCity.lat);
  const dx = to.x - from.x;
  const dy = to.y - from.y;
  const controlX = (from.x + to.x) / 2 - dy * 0.16;
  const controlY = (from.y + to.y) / 2 + dx * 0.16;
  return `M ${from.x.toFixed(1)} ${from.y.toFixed(1)} Q ${controlX.toFixed(1)} ${controlY.toFixed(1)} ${to.x.toFixed(1)} ${to.y.toFixed(1)}`;
}

function clampPan(pan: { x: number; y: number }, zoom: number) {
  const maxX = 36 + (zoom - 1) * VIEWBOX.width * 0.34;
  const maxY = 28 + (zoom - 1) * VIEWBOX.height * 0.34;
  return {
    x: Math.max(-maxX, Math.min(maxX, pan.x)),
    y: Math.max(-maxY, Math.min(maxY, pan.y)),
  };
}

export function OverviewChinaMap({
  navigate,
  copy: c,
  nodes,
  jobs,
  clusters,
}: {
  navigate: (
    page: Page,
    name?: string,
    options?: { query?: Record<string, string | undefined> },
  ) => void;
  copy: Copy;
  nodes: CRDNode[];
  jobs: Job[];
  clusters: Cluster[];
}) {
  const [features, setFeatures] = useState<Feature[]>([]);
  const [zoom, setZoom] = useState(1);
  const [pan, setPan] = useState({ x: 0, y: 0 });
  const [dragging, setDragging] = useState(false);
  const mapRef = useRef<HTMLDivElement>(null);
  const dragRef = useRef<{
    clientX: number;
    clientY: number;
    panX: number;
    panY: number;
  } | null>(null);
  const hasDraggedRef = useRef(false);
  const zh = c.nav.overview === "总览";

  const { cities, totalByCat } = useMemo(
    () => parseNodeLocations(nodes),
    [nodes],
  );
  const connections = useMemo(() => buildConnections(cities), [cities]);

  const totalNodes = nodes.length;
  const totalClusters = clusters.length;
  const totalJobs = jobs.length;
  const runningJobs = jobs.filter((j) => j.phase === "Running").length;

  const updateZoom = (nextZoom: number) => {
    const normalized = Math.max(1, Math.min(1.8, Number(nextZoom.toFixed(1))));
    setZoom(normalized);
    setPan((current) => clampPan(current, normalized));
  };

  const resetMap = () => {
    setZoom(1);
    setPan({ x: 0, y: 0 });
  };

  useEffect(() => {
    let alive = true;
    fetch("/china-provinces.geojson")
      .then((response) => response.json())
      .then((data: FeatureCollection) => {
        if (alive) {
          setFeatures(
            data.features.filter(
              (feature) => feature.properties?.adcode !== "100000_JD",
            ),
          );
        }
      })
      .catch(() => setFeatures([]));
    return () => {
      alive = false;
    };
  }, []);

  const provincePaths = useMemo(
    () =>
      features.map((feature) => ({
        name: feature.properties?.name ?? "",
        path: geometryPath(feature.geometry),
      })),
    [features],
  );

  const stats = [
    { key: "clusters", zh: "集群分布", en: "Clusters", value: totalClusters },
    { key: "nodes", zh: "节点分布", en: "Nodes", value: totalNodes },
    { key: "jobs", zh: "任务分布", en: "Jobs", value: totalJobs },
    {
      key: "cloud",
      zh: "云算力节点",
      en: "Cloud nodes",
      value: totalByCat.cloud,
    },
    { key: "edge", zh: "端算力节点", en: "Edge nodes", value: totalByCat.edge },
    {
      key: "robot",
      zh: "端真机节点",
      en: "Robot nodes",
      value: totalByCat.robot,
    },
  ];

  return (
    <section className="panel overview-china-panel">
      <div className="overview-china-heading">
        <div>
          <h3>{zh ? "资源与任务分布" : "Resource and task distribution"}</h3>
          <p>
            {zh
              ? "跨集群、跨地域、跨具身型号的运行态势"
              : "Runtime posture across clusters, regions and embodied models"}
          </p>
        </div>
        <div className="overview-china-legend">
          <span className="cloud">
            <CloudCog size={14} />
            {zh ? "云算力节点" : "Cloud"}
          </span>
          <span className="edge">
            <Server size={14} />
            {zh ? "端算力节点" : "Edge"}
          </span>
          <span className="robot">
            <Bot size={14} />
            {zh ? "端真机节点" : "Robot"}
          </span>
        </div>
      </div>

      <div className="overview-china-layout">
        <div
          className={`overview-china-map${dragging ? " is-dragging" : ""}`}
          ref={mapRef}
          onPointerDown={(event) => {
            if ((event.target as Element).closest(".overview-map-controls"))
              return;
            hasDraggedRef.current = false;
            dragRef.current = {
              clientX: event.clientX,
              clientY: event.clientY,
              panX: pan.x,
              panY: pan.y,
            };
            event.currentTarget.setPointerCapture(event.pointerId);
            setDragging(true);
          }}
          onPointerMove={(event) => {
            if (!dragRef.current || !mapRef.current) return;
            const rect = mapRef.current.getBoundingClientRect();
            const deltaX =
              (event.clientX - dragRef.current.clientX) *
              (VIEWBOX.width / rect.width);
            const deltaY =
              (event.clientY - dragRef.current.clientY) *
              (VIEWBOX.height / rect.height);
            if (Math.abs(deltaX) > 2 || Math.abs(deltaY) > 2)
              hasDraggedRef.current = true;
            setPan(
              clampPan(
                {
                  x: dragRef.current.panX + deltaX,
                  y: dragRef.current.panY + deltaY,
                },
                zoom,
              ),
            );
          }}
          onPointerUp={(event) => {
            dragRef.current = null;
            if (event.currentTarget.hasPointerCapture(event.pointerId))
              event.currentTarget.releasePointerCapture(event.pointerId);
            setDragging(false);
          }}
          onPointerCancel={() => {
            dragRef.current = null;
            setDragging(false);
          }}
        >
          <div
            className="overview-map-controls"
            aria-label={zh ? "地图缩放" : "Map zoom"}
          >
            <button
              type="button"
              aria-label={zh ? "放大地图" : "Zoom in"}
              disabled={zoom >= 1.8}
              onClick={() => updateZoom(zoom + 0.2)}
            >
              <Plus size={14} />
            </button>
            <button
              type="button"
              aria-label={zh ? "缩小地图" : "Zoom out"}
              disabled={zoom <= 1}
              onClick={() => updateZoom(zoom - 0.2)}
            >
              <Minus size={14} />
            </button>
            <button
              type="button"
              aria-label={zh ? "复位地图" : "Reset map"}
              disabled={zoom === 1 && pan.x === 0 && pan.y === 0}
              onClick={resetMap}
            >
              <RotateCcw size={13} />
            </button>
            <span>{Math.round(zoom * 100)}%</span>
          </div>
          <svg
            viewBox={`0 0 ${VIEWBOX.width} ${VIEWBOX.height}`}
            role="img"
            aria-label={
              zh ? "中国节点城市分布地图" : "China node city distribution map"
            }
          >
            <g
              className="china-map-viewport"
              transform={`translate(${pan.x} ${pan.y}) translate(${VIEWBOX.width / 2} ${VIEWBOX.height / 2}) scale(${zoom}) translate(${-VIEWBOX.width / 2} ${-VIEWBOX.height / 2})`}
            >
              <g className="china-provinces">
                {provincePaths.map((province, index) => (
                  <path key={`${province.name}-${index}`} d={province.path}>
                    <title>{province.name}</title>
                  </path>
                ))}
              </g>
              {cities.length >= 2 && (
                <g className="china-city-links" aria-hidden="true">
                  {connections.map(([from, to], index) => {
                    const path = connectionPath(cities, from, to);
                    return (
                      <g key={`${from}-${to}`}>
                        <path className="city-link-line" d={path} />
                        <circle
                          className={`link-particle particle-${index % 3}`}
                          r="2.7"
                        >
                          <animateMotion
                            path={path}
                            dur={`${2.8 + (index % 4) * 0.65}s`}
                            begin={`${index * -0.42}s`}
                            repeatCount="indefinite"
                          />
                        </circle>
                      </g>
                    );
                  })}
                </g>
              )}
              <g className="china-city-pins">
                {cities.map((city, index) => {
                  const point = project(city.lon, city.lat);
                  return (
                    <g
                      key={city.name}
                      className={`china-city-pin pin-${city.dominant}`}
                      transform={`translate(${point.x} ${point.y})`}
                      role="button"
                      tabIndex={0}
                      aria-label={`${city.name} ${city.nodes} ${zh ? "个节点" : "nodes"}`}
                      onClick={() => {
                        if (!hasDraggedRef.current)
                          navigate("clusters-nodes", undefined, {
                            query: { city: city.name },
                          });
                      }}
                      onKeyDown={(event) => {
                        if (event.key === "Enter" || event.key === " ")
                          navigate("clusters-nodes", undefined, {
                            query: { city: city.name },
                          });
                      }}
                    >
                      <circle className="pin-halo" r="14" />
                      <circle className="pin-core" r="6" />
                      <circle className="pin-badge" cx="11" cy="-11" r="10" />
                      <text
                        className="pin-value"
                        x="11"
                        y="-7.5"
                        textAnchor="middle"
                      >
                        {city.nodes}
                      </text>
                      <text
                        className="pin-label"
                        x="0"
                        y="27"
                        textAnchor="middle"
                      >
                        {city.name.replace("市", "")}
                      </text>
                      <g
                        className="china-city-tooltip"
                        transform={`translate(${point.x > 660 ? -134 : point.x < 135 ? 12 : -62} ${point.y < 80 ? 24 : -72})`}
                        pointerEvents="none"
                      >
                        <rect width="150" height="70" rx="9" />
                        <text className="tooltip-city" x="10" y="18">
                          {city.name}
                        </text>
                        <text className="tooltip-detail" x="10" y="36">
                          {city.nodes} {zh ? "个节点" : "nodes"} ·{" "}
                          {city.clusters} {zh ? "个集群" : "clusters"}
                        </text>
                        <text className="tooltip-detail" x="10" y="54">
                          {zh ? "云" : "Cloud"} {city.cloud} ·{" "}
                          {zh ? "端" : "Edge"} {city.edge} ·{" "}
                          {zh ? "真机" : "Robot"} {city.robot}
                        </text>
                      </g>
                    </g>
                  );
                })}
              </g>
            </g>
          </svg>
          {provincePaths.length === 0 && (
            <div className="overview-map-loading">
              {zh ? "地图加载中…" : "Loading map…"}
            </div>
          )}
          {cities.length === 0 && provincePaths.length > 0 && (
            <div className="overview-map-loading">
              {zh ? "暂无节点位置数据" : "No node location data"}
            </div>
          )}
          <div className="overview-map-caption">
            <Network size={15} />
            <span>
              <strong>{zh ? "全国调度视图" : "Nationwide scheduling"}</strong>
              <small>
                {zh
                  ? `覆盖 ${cities.length} 个城市`
                  : `${cities.length} cities`}
              </small>
            </span>
          </div>
        </div>

        <aside className="overview-china-aside">
          <div className="overview-map-stats">
            {stats.map((item) => (
              <button
                className={item.key}
                key={item.key}
                onClick={() =>
                  navigate(
                    item.key === "jobs"
                      ? "jobs"
                      : item.key === "clusters"
                        ? "clusters-management"
                        : "clusters-nodes",
                    undefined,
                    item.key === "cloud" ||
                      item.key === "edge" ||
                      item.key === "robot"
                      ? { query: { category: item.key } }
                      : undefined,
                  )
                }
              >
                <small>{zh ? item.zh : item.en}</small>
                <strong>{item.value}</strong>
              </button>
            ))}
          </div>
          <button
            className="overview-cross-region"
            onClick={() => navigate("jobs")}
          >
            <span>
              <Workflow size={15} />
              {zh ? "跨域任务" : "Cross-region jobs"}
            </span>
            <strong>
              {zh
                ? `${runningJobs} 个任务正在运行`
                : `${runningJobs} jobs running`}
            </strong>
            <small>
              {zh
                ? `覆盖 ${cities.length} 个城市、${totalClusters} 个集群`
                : `${cities.length} cities and ${totalClusters} clusters covered`}
            </small>
          </button>
          <div className="overview-city-summary">
            {cities.slice(0, 3).map((city) => (
              <button
                key={city.name}
                onClick={() =>
                  navigate("clusters-nodes", undefined, {
                    query: { city: city.name },
                  })
                }
              >
                <MapPin size={13} />
                <span>
                  <strong>{city.name}</strong>
                  <small>
                    {city.clusters} {zh ? "个集群" : "clusters"}
                  </small>
                </span>
                <b>{city.nodes}</b>
              </button>
            ))}
          </div>
        </aside>
      </div>
    </section>
  );
}
