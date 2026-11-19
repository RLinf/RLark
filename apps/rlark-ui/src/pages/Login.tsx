import { useEffect, useState, type FormEvent } from "react";
import { AlertCircle, ArrowRight, Eye, EyeOff } from "lucide-react";
import { storeAuthSession } from "../api";
import { authApi } from "../backend";
import type { Lang } from "../i18n";

export function UserLogin({
  onLogin,
  lang,
}: {
  onLogin: (username: string) => void;
  lang: Lang;
}) {
  const zh = lang === "zh";
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
      setError(zh ? "请输入账号和密码" : "Enter your username and password");
      return;
    }
    setLoading(true);
    setError("");
    authApi
      .login(username.trim(), password)
      .then((result) => {
        if (!result.token || result.role !== "user") {
          throw new Error(zh ? "登录响应无效" : "Invalid login response");
        }
        storeAuthSession(result.token, result.role);
        sessionStorage.setItem("rlark-user-auth", "1");
        sessionStorage.setItem("rlark-user-name", username.trim());
        onLogin(username.trim());
      })
      .catch((err) => {
        setError(
          err.status === 401
            ? zh
              ? "账号或密码错误"
              : "Incorrect username or password"
            : err.message,
        );
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
                src={`/rlark-logo-${zh ? "zh" : "en"}-light.png`}
                alt={
                  zh
                    ? "RLark 具身智能云原生纳管平台"
                    : "RLark cloud-native embodied intelligence platform"
                }
              />
            </div>
            <div className="user-login-heading">
              <h2>{zh ? "用户登录" : "User sign-in"}</h2>
              <p className="muted">
                {zh ? "请输入账号和密码" : "Enter your username and password"}
              </p>
            </div>
            <div className="admin-login-field">
              <label>{zh ? "账号" : "Username"}</label>
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
              <label>{zh ? "密码" : "Password"}</label>
              <div className="admin-login-password">
                <input
                  type={showPassword ? "text" : "password"}
                  value={password}
                  onChange={(e) => {
                    setPassword(e.target.value);
                    setError("");
                  }}
                  placeholder={zh ? "请输入密码" : "Enter your password"}
                  autoComplete="current-password"
                />
                <button
                  type="button"
                  className="admin-login-password-toggle"
                  aria-label={
                    showPassword
                      ? zh
                        ? "隐藏密码"
                        : "Hide password"
                      : zh
                        ? "显示密码"
                        : "Show password"
                  }
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
              <span>
                {loading
                  ? zh
                    ? "登录中…"
                    : "Signing in…"
                  : zh
                    ? "登录平台"
                    : "Sign in"}
              </span>
              {!loading && <ArrowRight size={16} />}
            </button>
            <a className="admin-login-back" href="/admin">
              <ArrowRight size={13} />
              {zh ? "管理后台" : "Admin console"}
            </a>
          </form>
        </div>
      </div>
    </div>
  );
}
