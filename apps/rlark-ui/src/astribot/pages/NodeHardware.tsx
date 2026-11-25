import { Cpu, HardDrive, MemoryStick, Network, Server, Thermometer, Zap } from "lucide-react";
import { formatChinaDateTime } from "../../utils/time.js";
import { formatHardwareBytes, formatHardwareValue, hardwareFieldLabel, type HardwareReading, type NodeHardwareSnapshot } from "../lib/mockNodeHardware";
import "./NodeHardware.css";

function Metric({ label, value, detail }: { label: string; value: string; detail?: string }) {
  return <div className="node-hardware-metric"><span>{label}</span><strong>{value}</strong>{detail && <small>{detail}</small>}</div>;
}

function CapabilityValue({ reading, format, zh }: { reading: HardwareReading<number>; format: (value: number | null) => string; zh: boolean }) {
  return <span className="node-hardware-capability"><strong>{format(reading.value)}</strong>{reading.capability !== "available" && <small>{hardwareFieldLabel(reading.capability, zh)}</small>}</span>;
}

function rate(value: number | null, zh: boolean) {
  if (value === null) return "—";
  const units = zh ? ["B/s", "KiB/s", "MiB/s", "GiB/s"] : ["B/s", "KiB/s", "MiB/s", "GiB/s"];
  let amount = value;
  let unit = 0;
  while (amount >= 1024 && unit < units.length - 1) { amount /= 1024; unit += 1; }
  return `${amount.toFixed(1)} ${units[unit]}`;
}

export function NodeHardware({ snapshot, zh }: { snapshot: NodeHardwareSnapshot; zh: boolean }) {
  const stale = snapshot.state === "stale";
  const title = zh ? "计算硬件" : "Compute hardware";
  const capacity = zh ? "容量" : "Capacity";
  const used = zh ? "已用" : "Used";
  const unavailable = zh ? "硬件指标暂不可用" : "Hardware metrics unavailable";

  return <section className="node-hardware" aria-label={title}>
    <header className="node-hardware-header">
      <div className="node-hardware-heading"><Server size={16} aria-hidden="true" /><h4>{title}</h4></div>
      <div className="node-hardware-meta">
        <span className="node-hardware-source">{snapshot.source}</span>
        {snapshot.observedAt && <span>{zh ? "采样于" : "Observed"} {formatChinaDateTime(snapshot.observedAt)}</span>}
      </div>
    </header>

    {snapshot.state === "error" ? <div className="node-hardware-message is-error" role="status">{snapshot.error ?? unavailable}</div> : <>
      {stale && <div className="node-hardware-message is-stale" role="status">{zh ? "数据已过期，以下为最近一次采样" : "Data is stale. Values below are from the last sample."}</div>}
      <div className="node-hardware-metrics">
        <section className="node-hardware-card" aria-label={zh ? "处理器" : "CPU"}>
          <h5><Cpu size={14} aria-hidden="true" />CPU</h5>
          <Metric label={zh ? "型号" : "Model"} value={snapshot.cpu.model || "—"} />
          <Metric label={zh ? "核心" : "Cores"} value={formatHardwareValue(snapshot.cpu.cores || null)} />
          <Metric label={zh ? "使用率" : "Utilization"} value={formatHardwareValue(snapshot.cpu.usagePercent, "%")} />
        </section>
        <section className="node-hardware-card" aria-label={zh ? "内存" : "Memory"}>
          <h5><MemoryStick size={14} aria-hidden="true" />{zh ? "内存" : "Memory"}</h5>
          <Metric label={capacity} value={formatHardwareBytes(snapshot.memory.capacityBytes || null)} />
          <Metric label={used} value={formatHardwareBytes(snapshot.memory.usedBytes)} />
          <Metric label={zh ? "使用率" : "Utilization"} value={formatHardwareValue(snapshot.memory.usagePercent, "%")} />
        </section>
        <section className="node-hardware-card node-hardware-wide" aria-label={zh ? "磁盘" : "Disks"}>
          <h5><HardDrive size={14} aria-hidden="true" />{zh ? "磁盘" : "Disks"}</h5>
          {snapshot.disks.length ? <div className="node-hardware-disk-list">{snapshot.disks.map((disk) => <div className="node-hardware-disk" key={disk.filesystem}>
            <code>{disk.filesystem}</code><span>{formatHardwareBytes(disk.usedBytes)} / {formatHardwareBytes(disk.capacityBytes)}</span>
          </div>)}</div> : <p className="node-hardware-empty">—</p>}
        </section>
        <section className="node-hardware-card" aria-label={zh ? "网络" : "Network"}>
          <h5><Network size={14} aria-hidden="true" />{zh ? "网络" : "Network"}</h5>
          <Metric label="RX" value={rate(snapshot.network.rxBytesPerSecond, zh)} />
          <Metric label="TX" value={rate(snapshot.network.txBytesPerSecond, zh)} />
        </section>
      </div>

      {snapshot.tegra && <section className="node-hardware-accelerator" aria-label="Orin GPU metrics">
        <header><h5><Zap size={14} aria-hidden="true" />{zh ? "Orin GPU" : "Orin GPU"}</h5><span className="node-hardware-source">source · {snapshot.tegra.source}</span></header>
        <div className="node-hardware-tegra-grid">
          <Metric label="GR3D" value={formatHardwareValue(snapshot.tegra.gr3dPercent, "%")} />
          <Metric label={zh ? "RAM" : "RAM"} value={snapshot.tegra.ramUsedBytes === null ? "—" : `${formatHardwareBytes(snapshot.tegra.ramUsedBytes)} / ${formatHardwareBytes(snapshot.tegra.ramTotalBytes)}`} />
          <Metric label={zh ? "温度" : "Temperature"} value={formatHardwareValue(snapshot.tegra.temperatureC, " °C")} />
          <Metric label="VDD_GPU" value={formatHardwareValue(snapshot.tegra.railPowerWatts, " W")} />
        </div>
      </section>}

      <section className="node-hardware-gpu" aria-label={zh ? "GPU 卡" : "GPU cards"}>
        <header className="node-hardware-gpu-heading">
          <h5><Zap size={14} aria-hidden="true" />GPU</h5>
          <span className="node-hardware-allocation">{zh ? "已分配 GPU 数（Pod requests）" : "Allocated GPUs (Pod requests)"}<strong>{snapshot.allocatedGPUs}</strong></span>
        </header>
        {snapshot.gpus.length ? <div className="node-hardware-table-wrap"><table>
          <thead><tr><th>{zh ? "卡" : "Card"}</th><th>{zh ? "型号" : "Model"}</th><th>{zh ? "显存" : "Memory"}</th><th>{zh ? "利用率" : "Utilization"}</th><th>{zh ? "温度" : "Temperature"}</th><th>{zh ? "功耗" : "Power"}</th></tr></thead>
          <tbody>{snapshot.gpus.map((gpu) => <tr key={gpu.index}>
            <td>GPU {gpu.index}</td><td>{gpu.model}</td>
            <td><CapabilityValue reading={gpu.memoryUsedBytes} format={(value) => `${formatHardwareBytes(value)} / ${formatHardwareBytes(gpu.memoryTotalBytes)}`} zh={zh} /></td>
            <td><CapabilityValue reading={gpu.utilizationPercent} format={(value) => formatHardwareValue(value, "%")} zh={zh} /></td>
            <td><CapabilityValue reading={gpu.temperatureC} format={(value) => formatHardwareValue(value, " °C")} zh={zh} /></td>
            <td><CapabilityValue reading={gpu.powerWatts} format={(value) => formatHardwareValue(value, " W")} zh={zh} /></td>
          </tr>)}</tbody>
        </table></div> : <div className="node-hardware-message is-neutral">{snapshot.state === "no-gpu" ? (zh ? "此节点未检测到 GPU" : "No GPU detected on this node") : (zh ? "未发现 GPU 卡" : "No GPU cards reported")}</div>}
        {snapshot.gpus.some((gpu) => gpu.temperatureC.capability === "unsupported" || gpu.powerWatts.capability === "unsupported") && <p className="node-hardware-footnote"><Thermometer size={13} aria-hidden="true" />{zh ? "DCGM 对此 GeForce 卡型仅提供有限字段；未提供的指标显示为 —。" : "DCGM exposes limited fields for this GeForce model. Unsupported metrics are shown as —."}</p>}
        <p className="node-hardware-footnote">{zh ? "Pod requests 是资源分配；GPU 利用率是设备实时活动，两者含义不同。" : "Pod requests show allocated capacity. GPU utilization is live device activity; they are different measures."}</p>
      </section>
    </>}
  </section>;
}
