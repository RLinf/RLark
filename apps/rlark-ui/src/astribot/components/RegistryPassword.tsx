import { useEffect, useRef, useState } from "react";
import { Copy as CopyIcon, Eye, EyeOff } from "lucide-react";
import { registryBrowseApi } from "../api";
import { copyText } from "../../utils/clipboard";
import "./RegistryPassword.css";

// A registry's password on its detail page: masked until asked for, read from the Gateway only then.
export function RegistryPassword({ registryID, zh }: { registryID: string; zh: boolean }) {
  const [password, setPassword] = useState<string | null>(null);
  const [passwordStatus, setPasswordStatus] = useState("");
  const [passwordLoading, setPasswordLoading] = useState(false);
  const passwordRequest = useRef<AbortController | null>(null);
  useEffect(() => {
    setPassword(null);
    setPasswordStatus("");
    setPasswordLoading(false);
    return () => passwordRequest.current?.abort();
  }, [registryID]);
  const revealPassword = async () => {
    if (password !== null) {
      setPassword(null);
      setPasswordStatus("");
      return;
    }
    setPasswordLoading(true);
    const controller = new AbortController();
    passwordRequest.current = controller;
    try {
      const result = await registryBrowseApi.password(registryID, controller.signal);
      if (!controller.signal.aborted) {
        setPassword(result.password);
        setPasswordStatus("");
      }
    } catch {
      if (!controller.signal.aborted) setPasswordStatus(zh ? "无法读取密码" : "Unable to read password");
    } finally {
      if (!controller.signal.aborted) setPasswordLoading(false);
    }
  };
  return (
    <>
      <div className="registry-password-row">
        <strong>{password ?? "••••••••"}</strong>
        <div className="registry-password-actions">
          <button type="button" className="registry-password-button" disabled={passwordLoading} onClick={() => void revealPassword()} aria-label={password !== null ? (zh ? "隐藏密码" : "Hide password") : (zh ? "显示密码" : "Show password")} title={password !== null ? (zh ? "隐藏密码" : "Hide password") : (zh ? "显示密码" : "Show password")}>
            {password !== null ? <EyeOff size={16} /> : <Eye size={16} />}
          </button>
          {password !== null && <button type="button" className="registry-password-button" aria-label={zh ? "复制密码" : "Copy password"} title={zh ? "复制密码" : "Copy password"} onClick={() => void copyText(password).then(ok => setPasswordStatus(ok ? (zh ? "已复制" : "Copied") : (zh ? "复制失败" : "Copy failed"))).catch(() => setPasswordStatus(zh ? "复制失败" : "Copy failed"))}><CopyIcon size={16} /></button>}
        </div>
      </div>
      {passwordStatus && <span role="status">{passwordStatus}</span>}
    </>
  );
}
