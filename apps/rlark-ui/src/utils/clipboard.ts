// copyText 把文本写入系统剪贴板，并返回是否复制成功。
//
// 浏览器只在安全上下文（HTTPS 或 localhost）下暴露 navigator.clipboard，
// 控制台经 HTTP 访问时该对象为 undefined，直接调用会抛 TypeError。
// 因此这里先尝试异步 Clipboard API，失败或不可用时回退到
// document.execCommand("copy")，保证两种部署方式下复制都可用。
export async function copyText(value: string): Promise<boolean> {
  if (!value) return false;

  try {
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(value);
      return true;
    }
  } catch {
    // Fall through for browsers that block Clipboard API on local HTTP pages.
  }

  const textarea = document.createElement("textarea");
  textarea.value = value;
  textarea.style.position = "fixed";
  textarea.style.opacity = "0";
  document.body.appendChild(textarea);
  textarea.focus({ preventScroll: true });
  textarea.select();
  textarea.setSelectionRange(0, value.length);
  const copied = document.execCommand("copy");
  textarea.remove();
  return copied;
}
