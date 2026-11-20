import { useState } from "react";
import "./provision.css";

// Under a cluster's making or an SSH join that does not run: retry from the stage it stopped at, or clear it. The
// account and password typed here are sent with that one request and forgotten.
export function RetryClear({ zh, view, user, pending, run, retry, clear, clearNote, clearLabel, clearReach, goneWhy = "" }: {
  zh: boolean;
  view: { canRetry: boolean; retryNeedsPassword: boolean; canClear: boolean; clearReachesMachine: boolean };
  // The account the machine was logged in to with, unless another is typed.
  user: string;
  pending: boolean;
  run: (work: () => Promise<unknown>) => Promise<void>;
  retry: (user: string, password: string) => Promise<unknown>;
  clear: (user: string, password: string, machineGone: boolean) => Promise<unknown>;
  clearNote: string;
  clearLabel: string;
  // What clearing logs in to the machine for, and (a cluster's machine) how it came to be gone.
  clearReach: string;
  goneWhy?: string;
}) {
  const [retryUser, setRetryUser] = useState("");
  const [password, setPassword] = useState("");
  const [machineGone, setMachineGone] = useState(false);
  const send = (work: () => Promise<unknown>) => run(async () => {
    await work();
    setPassword("");
    setRetryUser("");
    setMachineGone(false);
  });

  return <>
    {view.canRetry && <form className="cert-form provision-retry" onSubmit={(event) => { event.preventDefault(); void send(() => retry(retryUser || user, password)); }}>
      {view.retryNeedsPassword && <label>
        <span>{zh ? "SSH 账号" : "SSH account"}</span>
        <input value={retryUser || user} onChange={(event) => setRetryUser(event.target.value)} />
      </label>}
      {view.retryNeedsPassword && <label>
        <span>{zh ? "SSH 密码（这一步要重新登录机器）" : "SSH password (this stage logs in again)"}</span>
        <input type="password" autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} />
      </label>}
      <button type="submit" className="primary-button" disabled={pending || (view.retryNeedsPassword && !password)}>
        {zh ? "从这一步重试" : "Retry from this stage"}
      </button>
    </form>}
    {view.canClear && <div className="provision-clear">
      <small>{clearNote}</small>
      <div className="cert-form">
        {view.clearReachesMachine && !view.retryNeedsPassword && !machineGone && <label>
          <span>{zh ? `SSH 密码（${clearReach}）` : `SSH password (${clearReach})`}</span>
          <input type="password" autoComplete="new-password" value={password} onChange={(event) => setPassword(event.target.value)} />
        </label>}
        {view.clearReachesMachine && <label className="provision-clear-gone">
          <input type="checkbox" checked={machineGone} onChange={(event) => setMachineGone(event.target.checked)} />
          <span>{zh ? `机器已经不在了${goneWhy}：只清 RLark 里的记录` : `The machine is gone${goneWhy}: clear RLark's records only`}</span>
        </label>}
        <button type="button" className="secondary-button danger" disabled={pending || (view.clearReachesMachine && !machineGone && !password)}
          onClick={() => void send(() => clear(retryUser || user, machineGone ? "" : password, machineGone))}>
          {clearLabel}
        </button>
      </div>
    </div>}
  </>;
}
