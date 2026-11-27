/** Mount profile and read-only switch of a Job volume that names a storage. */
export function StorageMountOptions({
  zh,
  profile,
  readOnly,
  onProfile,
  onReadOnly,
}: {
  zh: boolean;
  profile: string;
  readOnly: boolean;
  onProfile: (profile: string) => void;
  onReadOnly: (readOnly: boolean) => void;
}) {
  return (
    <>
      <label className="mount-field-box mount-size-field">
        <span>{zh ? "挂载档位" : "Mount profile"}</span>
        <select value={profile} onChange={(e) => onProfile(e.target.value)}>
          <option value="">{zh ? "存储的默认档位" : "Storage default"}</option>
          <option value="default">default</option>
          <option value="read">{zh ? "read（只读）" : "read (read-only)"}</option>
          <option value="archive">
            {zh ? "archive（关写缓存）" : "archive (write cache off)"}
          </option>
        </select>
      </label>
      <label className="mount-field-box mount-size-field">
        <span>{zh ? "只读" : "Read-only"}</span>
        <input
          type="checkbox"
          checked={readOnly}
          onChange={(e) => onReadOnly(e.target.checked)}
        />
      </label>
    </>
  );
}
