import { useEffect, useState, type FormEvent } from "react";
import { AlertCircle, ArrowRight, Eye, EyeOff } from "lucide-react";

export function UserLogin({
  onLogin,
}: {
  onLogin: (username: string) => void;
}) {
  const [username, setUsername] = useState("user");
  const [password, setPassword] = useState("");
  const [showPassword, setShowPassword] = useState(false);
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!error) return;
    const timer = window.setTimeout(() => setError(""), 4000);
    return () => window.clearTimeout(timer);
  }, [error]);

  const handleSubmit = (e: FormEvent) => {
    e.preventDefault();
    if (!username.trim() || !password.trim()) {
      setError("请输入账号和密码");
      return;
    }
    setLoading(true);
    setError("");
    fetch("/api/v1/auth/login", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ username: username.trim(), password }),
    })
      .then((resp) =>
        resp.ok
          ? resp.json()
          : Promise.reject(
              new Error(
                resp.status === 401 ? "账号或密码错误" : `HTTP ${resp.status}`,
              ),
            ),
      )
      .then(() => {
        sessionStorage.setItem("rlark-user-auth", "1");
        sessionStorage.setItem("rlark-user-name", username.trim());
        onLogin(username.trim());
      })
      .catch((err) => {
        setError(err.message);
        setLoading(false);
      });
  };

  return (
    <div className="admin-login-page theme-light">
      <div className="user-login-orb user-login-orb-one" />
      <div className="user-login-orb user-login-orb-two" />
      <div className="admin-login-body">
        <div className="admin-login-panel">
          <form className="admin-login-card" onSubmit={handleSubmit}>
            <div className="user-login-brand">
              <img
                className="user-login-brand-logo"
                src="/rlark-logo-zh-light.png"
                alt="RLark 具身智能云原生纳管平台"
              />
            </div>
            <div className="user-login-heading">
              <h2>用户登录</h2>
              <p className="muted">请输入账号和密码</p>
            </div>
            <div className="admin-login-field">
              <label>账号</label>
              <input
                type="text"
                value={username}
                onChange={(e) => {
                  setUsername(e.target.value);
                  setError("");
                }}
                placeholder="user"
                autoComplete="username"
              />
            </div>
            <div className="admin-login-field">
              <label>密码</label>
              <div className="admin-login-password">
                <input
                  type={showPassword ? "text" : "password"}
                  value={password}
                  onChange={(e) => {
                    setPassword(e.target.value);
                    setError("");
                  }}
                  placeholder="请输入密码"
                  autoComplete="current-password"
                />
                <button
                  type="button"
                  className="admin-login-password-toggle"
                  aria-label={showPassword ? "隐藏密码" : "显示密码"}
                  aria-pressed={showPassword}
                  onClick={() => setShowPassword((visible) => !visible)}
                >
                  {showPassword ? <EyeOff size={18} /> : <Eye size={18} />}
                </button>
              </div>
            </div>
            {error && (
              <div
                className="login-inline-error"
                role="alert"
                aria-live="assertive"
              >
                <AlertCircle size={14} />
                <span>{error}</span>
              </div>
            )}
            <button
              type="submit"
              className="primary-button admin-login-btn"
              disabled={loading}
            >
              <span>{loading ? "登录中…" : "登录平台"}</span>
              {!loading && <ArrowRight size={16} />}
            </button>
            <a className="admin-login-back" href="/admin">
              <ArrowRight size={13} />
              管理后台
            </a>
          </form>
        </div>
      </div>
    </div>
  );
}
