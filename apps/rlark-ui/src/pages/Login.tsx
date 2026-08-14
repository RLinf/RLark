import { useState, type FormEvent } from "react";
import { ArrowRight, Shield } from "lucide-react";

export function UserLogin({
  onLogin,
}: {
  onLogin: (username: string) => void;
}) {
  const [username, setUsername] = useState("user");
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [loading, setLoading] = useState(false);

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
      <div className="admin-login-body">
        <form className="admin-login-card" onSubmit={handleSubmit}>
          <div className="admin-login-logo">
            <Shield size={32} />
          </div>
          <h2>用户登录</h2>
          <p className="muted">请输入账号和密码</p>
          <div className="admin-login-field">
            <label>账号</label>
            <input
              type="text"
              value={username}
              onChange={(e) => setUsername(e.target.value)}
              placeholder="user"
              autoComplete="username"
            />
          </div>
          <div className="admin-login-field">
            <label>密码</label>
            <input
              type="password"
              value={password}
              onChange={(e) => setPassword(e.target.value)}
              placeholder="••••••••"
              autoComplete="current-password"
            />
          </div>
          {error && (
            <div className="cert-error" style={{ marginBottom: 12 }}>
              {error}
            </div>
          )}
          <button
            type="submit"
            className="primary-button admin-login-btn"
            disabled={loading}
          >
            {loading ? "登录中…" : "登录"}
          </button>
          <a className="admin-login-back" href="/admin">
            <ArrowRight size={13} />
            管理后台
          </a>
        </form>
      </div>
    </div>
  );
}
