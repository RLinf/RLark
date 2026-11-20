import type { Copy } from "../../i18n";
import { storageCopy } from "../i18n";
import type { StorageClassFormState } from "../lib/storageClass";

type SmbStorageFormFields = Pick<StorageClassFormState, "host" | "share" | "sub_path" | "domain" | "username" | "password">;

/** Connection fields of an SMB/NAS storage class. */
export function SmbStorageFields({
  copy: c,
  zh,
  isEdit,
  value,
  onChange,
}: {
  copy: Copy;
  zh: boolean;
  isEdit: boolean;
  value: SmbStorageFormFields;
  onChange: (patch: Partial<SmbStorageFormFields>) => void;
}) {
  const s = storageCopy(c);
  return (
    <div className="form-section">
      <strong>{s.connection}</strong>
      <div className="form-grid">
        <label>
          {s.host} *
          <input
            required
            value={value.host}
            onChange={(e) => onChange({ host: e.target.value })}
            placeholder="10.0.0.10"
          />
        </label>
        <label>
          {s.share} *
          <input
            required
            value={value.share}
            onChange={(e) => onChange({ share: e.target.value })}
            placeholder="shared"
          />
        </label>
        <label>
          {s.subPath}
          <input
            value={value.sub_path}
            onChange={(e) => onChange({ sub_path: e.target.value })}
            placeholder={zh ? "可选，例如 team/data" : "Optional, e.g. team/data"}
          />
        </label>
        <label>
          {s.domain}
          <input
            value={value.domain}
            onChange={(e) => onChange({ domain: e.target.value })}
            placeholder={
              zh ? "可选，例如 corp.example.com" : "Optional, e.g. corp.example.com"
            }
          />
        </label>
        <label>
          {s.username} *
          <input
            required
            autoComplete="off"
            value={value.username}
            onChange={(e) => onChange({ username: e.target.value })}
          />
        </label>
        <label>
          {s.password} *
          <input
            required={!isEdit}
            type="password"
            autoComplete="new-password"
            value={value.password}
            onChange={(e) => onChange({ password: e.target.value })}
            placeholder={
              isEdit
                ? zh
                  ? "留空则保持原密码"
                  : "Leave blank to keep the existing password"
                : "••••••••••••"
            }
          />
        </label>
      </div>
      <p className="muted">{s.smbHint}</p>
    </div>
  );
}
