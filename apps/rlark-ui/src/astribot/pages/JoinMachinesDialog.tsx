import { useEffect, useRef, useState, type KeyboardEvent as ReactKeyboardEvent } from "react";
import { createPortal } from "react-dom";
import { KeyRound, ShieldCheck, X } from "lucide-react";
import { overlayContainer } from "../../utils/overlayContainer";
import { ApiError } from "../../api";
import { machinesApi, nodeJoinsApi, robotModelsApi, type RobotModelItem } from "../api";
import { useAction } from "../components/useAction";
import { joinCredentialGroups, joinCredentialProblem, type JoinCredentialInput } from "../lib/machineJoin";
import { sshJoinFailure, type AdmittedMachine, type Machine, type MachineList } from "../lib/siteNodeDisplay";
import "./JoinMachinesDialog.css";

// Confirms the batch before admitting it: one password block per NodeClass, because every machine of a class logs in
// with the same accounts. A stored password may be kept by leaving the field empty; a new one is written to the class
// and used for this join. Nothing typed here is read back.
export function JoinMachinesDialog({ zh, clusterID, clusterName, machines, classes, labels, canSubmitMachines, onClose, onJoined, onUseCode }: {
  zh: boolean;
  clusterID: string;
  clusterName: string;
  machines: Machine[];
  classes: MachineList["classes"];
  labels: Record<string, string>;
  canSubmitMachines: () => boolean;
  onUseCode: () => void;
  onClose: () => void;
  onJoined: () => Promise<unknown> | void;
}) {
  const groups = joinCredentialGroups(machines, classes);
  const [credentials, setCredentials] = useState<Record<string, JoinCredentialInput>>({});
  const [answers, setAnswers] = useState<AdmittedMachine[]>([]);
  const [problem, setProblem] = useState("");
  const [machineType, setMachineType] = useState<"Robot" | "Server" | "Unknown">(() => machines.every((machine) => machine.machine_type === "Robot") ? "Robot" : machines.length === 1 && machines[0].machine_type === "Server" ? "Server" : "Unknown");
  const [nodeClass, setNodeClass] = useState(machines.length === 1 ? machines[0].node_class : "");
  const [customModel, setCustomModel] = useState("");
  const [models, setModels] = useState<RobotModelItem[]>([]);
  const [server, setServer] = useState({ user: "", password: "", sudo: "", nodeName: "" });
  const [offerCode, setOfferCode] = useState(false);
  const [relayRetrying, setRelayRetrying] = useState(false);
  const { pending, error: actionError, setError, run } = useAction();
  const dialogRef = useRef<HTMLElement>(null);
  const passwordRef = useRef<HTMLInputElement>(null);
  const label = (name: string) => name === "s1u" ? "S1U" : name;
  const robotClasses = Object.entries(classes).filter(([, item]) => item.kind === "Robot" || Boolean(item.model || item.labels?.["rlark.io/robot-model"]));
  const selectedClass = classes[nodeClass];
  const selectedMachine = machines.length === 1 ? machines[0] : undefined;
  const model = machines.length === 1 ? selectedClass?.model || selectedClass?.labels?.["rlark.io/robot-model"] || selectedMachine?.machine_model || "" : "";
  const selectedRobotModel = model || customModel;
  const observedModel = selectedMachine?.observed_identity?.model;
  const observedModelKnown = !observedModel || models.some((item) => item.name === observedModel);

  useEffect(() => { void robotModelsApi.list().then((result) => setModels(result.items)).catch(() => setModels([])); }, []);
  useEffect(() => {
    setRelayRetrying(false);
    if (!pending || machineType !== "Server") return;
    const timer = window.setTimeout(() => setRelayRetrying(true), 10000);
    return () => window.clearTimeout(timer);
  }, [pending, machineType]);

  useEffect(() => {
    const scroller = document.querySelector<HTMLElement>(".main-area");
    const previous = scroller?.style.overflow;
    if (scroller) scroller.style.overflow = "hidden";
    return () => {
      if (scroller && previous !== undefined) scroller.style.overflow = previous;
    };
  }, []);

  useEffect(() => {
    const closeOnEscape = (event: KeyboardEvent) => {
      if (event.key === "Escape" && !pending) onClose();
    };
    document.addEventListener("keydown", closeOnEscape);
    return () => document.removeEventListener("keydown", closeOnEscape);
  }, [onClose, pending]);

  useEffect(() => {
    passwordRef.current?.focus();
  }, []);

  const trapFocus = (event: ReactKeyboardEvent<HTMLElement>) => {
    if (event.key !== "Tab") return;
    const candidates = dialogRef.current?.querySelectorAll<HTMLElement>(
      "button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), a[href], [tabindex]:not([tabindex='-1'])",
    );
    const focusable = candidates ? [...candidates].filter((element) => element.getClientRects().length > 0) : [];
    if (!focusable.length) return;
    const first = focusable[0];
    const last = focusable[focusable.length - 1];
    if (event.shiftKey && document.activeElement === first) {
      event.preventDefault();
      last.focus();
    } else if (!event.shiftKey && document.activeElement === last) {
      event.preventDefault();
      first.focus();
    }
  };

  const update = (name: string, patch: Partial<JoinCredentialInput>) => {
    setCredentials((current) => ({
      ...current,
      [name]: { ...(current[name] ?? { password: "", sudo: "" }), ...patch },
    }));
    setProblem("");
    setError("");
  };

  const submit = () => {
    if (pending) return;
    if (!canSubmitMachines()) {
      setProblem(zh ? "所选机器的发现记录已过期或已不可加入。请返回列表重新选择。" : "A selected machine is no longer recently seen or joinable. Return to the list and select again.");
      return;
    }
    if (machineType === "Unknown") {
      setProblem(zh ? "请选择机器人或普通服务器。" : "Choose Robot or Server.");
      return;
    }
    if (machineType === "Robot") {
      const found = joinCredentialProblem(groups, credentials, zh);
      if (found) { setProblem(found); return; }
      if (machines.length === 1 && (!nodeClass || !selectedRobotModel)) {
        setProblem(zh ? "请选择机器人配置和型号。" : "Select a robot class profile and model.");
        return;
      }
      if (machines.some((machine) => machine.machine_type === "Server")) {
        setProblem(zh ? "请按类型筛选后单独加入机器人或服务器。" : "Filter by type and submit Robot and Server candidates separately.");
        return;
      }
    }
    setProblem("");
    setAnswers([]);
    setOfferCode(false);
    void run(async () => {
      if (machineType === "Server") {
        if (!selectedMachine || !server.user.trim() || !server.password || !selectedMachine.host_key || !selectedMachine.host_key_algorithm) {
          setProblem(zh ? "单独选择一台带有已扫描 SSH 主机密钥的机器，并填写账号和口令。" : "Select one machine with a scanned SSH host key and enter its account and password.");
          return;
        }
        try {
          await nodeJoinsApi.create({
            cluster_id: clusterID,
            machine_name: selectedMachine.name,
            machine_type: "Server",
            ...(selectedMachine.machine_model ? { machine_model: selectedMachine.machine_model } : {}),
            address: selectedMachine.address ?? "",
            port: selectedMachine.port ?? 22,
            host_key: selectedMachine.host_key,
            host_key_algorithm: selectedMachine.host_key_algorithm,
            user: server.user.trim(),
            password: server.password,
            ...(server.sudo ? { sudo_password: server.sudo } : {}),
            ...(server.nodeName.trim() ? { node_name: server.nodeName.trim() } : {}),
          });
          await onJoined();
          onClose();
        } catch (reason) {
          const reasonName = reason instanceof ApiError ? (reason.body as { reason?: string } | undefined)?.reason : undefined;
          if (reasonName && ["Unreachable", "RelayUnavailable", "RelayRejected"].includes(reasonName)) setOfferCode(true);
          throw new Error(sshJoinFailure(reasonName, (reason as Error).message, zh));
        } finally { setServer((current) => ({ ...current, password: "", sudo: "" })); }
        return;
      }
      for (const group of groups) {
        const typed = credentials[group.name];
        if (!typed?.password.trim()) continue;
        await machinesApi.setClassAccess(group.name, {
          node_password: typed.password,
          ...(typed.sudo.trim() ? { sudo_password: typed.sudo } : {}),
        });
      }
      const result = await machinesApi.admit({
        cluster_id: clusterID,
        machines: machines.map((machine) => machine.name),
        machine_type: "Robot",
        ...(machines.length === 1 ? { node_class: nodeClass } : {}),
        ...(selectedRobotModel ? { machine_model: selectedRobotModel } : {}),
        ...(Object.keys(labels).length ? { labels } : {}),
      });
      setAnswers(result.machines);
      await onJoined();
      if (result.machines.every((answer) => answer.admitted)) onClose();
    });
  };

  const machineLabel = (name: string) => {
    const machine = machines.find((item) => item.name === name);
    return machine?.serial || machine?.address || name;
  };

  return createPortal(
    <div className="modal-backdrop join-machines-backdrop">
      <section
        ref={dialogRef}
        className="modal join-machines-modal"
        role="dialog"
        aria-modal="true"
        aria-labelledby="join-machines-title"
        aria-describedby="join-machines-description"
        aria-busy={pending}
        onKeyDown={trapFocus}
      >
        <header className="join-machines-head">
          <span className="join-machines-icon"><KeyRound size={19} aria-hidden="true" /></span>
          <div className="join-machines-heading">
            <span className="eyebrow">{zh ? "确认入网" : "Confirm join"}</span>
            <h2 id="join-machines-title">{zh ? `加入 ${machines.length} 台机器` : `Join ${machines.length} ${machines.length === 1 ? "machine" : "machines"}`}</h2>
            <p id="join-machines-description">{zh ? "目标集群" : "Target cluster"} <strong>{clusterName}</strong>{zh ? " · 口令按机型分组填写。" : " · Passwords are entered once per NodeClass."}</p>
          </div>
          <button type="button" className="icon-button" onClick={onClose} disabled={pending} aria-label={zh ? "关闭" : "Close"}><X size={17} /></button>
        </header>

        <div className="modal-body join-machines-body">
          <div className="join-machines-selected">
            {machines.map((machine) => <span className="join-machines-chip" key={machine.name}>{machine.serial || machine.address || machine.name}</span>)}
          </div>

          <label className="join-machines-field"><span>{zh ? "机器类型" : "Machine type"}</span>
            <select value={machineType} onChange={(event) => { setMachineType(event.target.value as typeof machineType); setProblem(""); }} disabled={pending}>
              <option value="Unknown">{zh ? "选择类型" : "Choose type"}</option>
              <option value="Robot">Robot · {zh ? "机器人（机型预设）" : "robot profile"}</option>
              <option value="Server" disabled={machines.length !== 1}>Server · {zh ? "普通服务器（SSH）" : "server over SSH"}</option>
            </select>
          </label>

          {machineType === "Robot" && machines.length === 1 && <>
            <label className="join-machines-field"><span>{zh ? "机器人配置" : "Robot class profile"}</span>
              <select value={nodeClass} onChange={(event) => setNodeClass(event.target.value)} disabled={pending}>
                <option value="">{zh ? "选择 NodeClass" : "Select NodeClass"}</option>
                {robotClasses.map(([name, item]) => <option value={name} key={name}>{label(name)}{item.model ? ` · ${item.model}` : ""}</option>)}
              </select>
            </label>
            <label className="join-machines-field"><span>{zh ? "机器人型号" : "Robot model"}</span>
              {model ? <input value={model} readOnly aria-label={zh ? "NodeClass 型号" : "NodeClass model"} /> : <select value={customModel} onChange={(event) => setCustomModel(event.target.value)} disabled={pending}>
                <option value="">{zh ? "选择型号" : "Select model"}</option>{models.map((item) => <option value={item.name} key={item.name}>{item.displayName || item.name}</option>)}
              </select>}
              {observedModel && !observedModelKnown && <small className="join-machines-error">{zh ? `观测型号 ${observedModel} 不在当前型号目录中。` : `Observed model ${observedModel} is not in the current model catalog.`}</small>}
              {observedModel && selectedRobotModel && observedModel !== selectedRobotModel && <small className="join-machines-error">{zh ? `配置型号 ${selectedRobotModel} 与观测型号 ${observedModel} 不匹配。` : `Selected model ${selectedRobotModel} does not match observed model ${observedModel}.`}</small>}
            </label>
          </>}

          {machineType === "Server" && selectedMachine && <section className="join-machines-class">
            <div className="join-machines-class-head">
              <strong>{selectedMachine.serial || selectedMachine.name}</strong>
              <span className="mono">{selectedMachine.address}:{selectedMachine.port ?? 22}</span>
            </div>
            <small className="join-machines-fingerprint">{selectedMachine.host_key_algorithm} · {selectedMachine.host_key}</small>
            <p className="join-machines-hint">{zh ? "直连失败时，Gateway 会通过站点中继重试。" : "If direct SSH is unreachable, Gateway retries through the site relay."}</p>
            {pending && <p className="site-network-inline-status" role="status">{relayRetrying ? (zh ? "直连失败，正在通过站点中继重试" : "Direct SSH failed. Retrying through the site relay.") : (zh ? "正在尝试直连 SSH…" : "Trying direct SSH…")}</p>}
            <div className="join-machines-server-grid">
              <label className="join-machines-field"><span>{zh ? "SSH 账号" : "SSH account"}</span><input value={server.user} onChange={(event) => setServer((current) => ({ ...current, user: event.target.value }))} disabled={pending} /></label>
              <label className="join-machines-field"><span>{zh ? "SSH 口令" : "SSH password"}</span><input type="password" autoComplete="new-password" value={server.password} onChange={(event) => setServer((current) => ({ ...current, password: event.target.value }))} disabled={pending} /></label>
              <label className="join-machines-field"><span>{zh ? "sudo 口令（可选）" : "sudo password (optional)"}</span><input type="password" autoComplete="new-password" value={server.sudo} onChange={(event) => setServer((current) => ({ ...current, sudo: event.target.value }))} disabled={pending} /></label>
              <label className="join-machines-field"><span>{zh ? "节点名（可选）" : "Node name (optional)"}</span><input value={server.nodeName} onChange={(event) => setServer((current) => ({ ...current, nodeName: event.target.value }))} disabled={pending} /></label>
            </div>
          </section>}

          {machineType === "Robot" && groups.map((group, index) => {
            const typed = credentials[group.name];
            const stored = Boolean(group.access?.node.configured);
            return <section className="join-machines-class" key={group.name}>
              <div className="join-machines-class-head"><strong>{label(group.name)}</strong><span>{zh ? `${group.machines.length} 台` : `${group.machines.length} machines`}</span></div>
              <div className="join-machines-credential">
                <span><ShieldCheck size={14} />{zh ? "Orin 入口口令" : "Entry-device password"}</span>
                <span className={`status ${group.access?.entry.configured ? "status-online" : "status-failed"}`}><i />{group.access?.entry.configured ? (zh ? "已配置" : "Configured") : (zh ? "未配置" : "Not configured")}</span>
              </div>
              <div className="join-machines-credential">
                <span><KeyRound size={14} />{zh ? "x86 登录口令" : "x86 password"}</span>
                <span className={`status ${stored ? "status-online" : "status-pending"}`}><i />{stored ? (zh ? "已保存" : "Stored") : (zh ? "未配置" : "Not configured")}</span>
              </div>
              <label className="join-machines-field">
                <span>{zh ? "x86 登录口令" : "x86 login password"}</span>
                <input
                  ref={index === 0 ? passwordRef : undefined}
                  type="password"
                  value={typed?.password ?? ""}
                  maxLength={256}
                  autoComplete="new-password"
                  disabled={pending}
                  placeholder={stored ? (zh ? "已保存；留空继续使用" : "Stored; leave empty to keep it") : (zh ? "必填：该机型还没有口令" : "Required: no password is stored")}
                  onChange={(event) => update(group.name, { password: event.target.value })}
                />
                <small>{stored ? (zh ? "留空使用已保存的机型凭据；填写会覆盖并保存。" : "Leave empty to use the stored class credential; typing a value replaces and saves it.") : (zh ? "该机型还没有 x86 登录口令，填写后才能加入。" : "This class has no x86 password yet; enter one to join.")}</small>
              </label>
              <label className="join-machines-field">
                <span>{zh ? "sudo 口令（可选）" : "sudo password (optional)"}</span>
                <input
                  type="password"
                  value={typed?.sudo ?? ""}
                  maxLength={256}
                  autoComplete="new-password"
                  disabled={pending}
                  placeholder={zh ? "与登录口令相同则留空" : "Same as the login password when empty"}
                  onChange={(event) => update(group.name, { sudo: event.target.value })}
                />
              </label>
            </section>;
          })}

          {machineType === "Robot" && <div className="join-machines-note">
            <KeyRound size={15} />
            <span>{zh ? "口令保存为该机型的凭据；RLark 只显示是否已录入，不显示内容。下次同机型可留空。" : "The password is saved as the class credential. RLark only shows whether it is stored, never its value. Later joins of the class can leave it empty."}</span>
          </div>}
          {machineType === "Server" && <div className="join-machines-note">
            <KeyRound size={15} />
            <span>{zh ? "SSH 口令仅用于此次远程入网，RLark 不会保存。" : "The SSH password is used for this join only and is not stored."}</span>
          </div>}

          {problem && <div className="join-machines-error" role="alert">{problem}</div>}
          {actionError && <div className="join-machines-error" role="alert">{actionError}</div>}
          {offerCode && <button type="button" className="secondary-button" onClick={onUseCode}>{zh ? "改用入网码" : "Use a join code"}</button>}
          {answers.some((answer) => !answer.admitted) && <div className="join-machines-results" role="alert">
            <strong>{zh ? "部分机器没有提交成功" : "Some machines were not admitted"}</strong>
            {answers.filter((answer) => !answer.admitted).map((answer) => <span key={answer.name}><b>{machineLabel(answer.name)}</b>：{answer.error}</span>)}
          </div>}
          {answers.length > 0 && answers.every((answer) => answer.admitted) && <div className="join-machines-success" role="status">{zh ? "已提交，入网在服务端继续。" : "Submitted. The join continues on the server."}</div>}
        </div>

        <footer className="join-machines-foot">
          <span>{zh ? "密码不会出现在 URL 或日志中。" : "Passwords never appear in a URL or the logs."}</span>
          <div>
            <button type="button" className="secondary-button" onClick={onClose} disabled={pending}>{zh ? "取消" : "Cancel"}</button>
            <button type="button" className="primary-button" onClick={() => void submit()} disabled={pending || machineType === "Unknown" || machineType === "Robot" && groups.length === 0}>{pending ? (zh ? "提交中…" : "Submitting…") : (zh ? "确认加入" : "Confirm join")}</button>
          </div>
        </footer>
      </section>
    </div>,
    overlayContainer(),
  );
}
