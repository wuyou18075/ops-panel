// 复制到剪贴板：安全上下文优先异步 API，否则用 execCommand 兜底。
// 兜底时把 textarea 挂到给定容器（通常是模态框 DOM）内，绕过 Naive UI
// NModal 的焦点陷阱——否则 textarea 挂在 body 上会被焦点陷阱夺回，选区失效，
// execCommand 复制到错误/旧内容（原「复制的不是生成的命令」bug）。
export async function copyToClipboard(text: string, container?: HTMLElement): Promise<boolean> {
  if (!text) return false;
  if (navigator.clipboard && window.isSecureContext) {
    try {
      await navigator.clipboard.writeText(text);
      return true;
    } catch {
      /* 落到 execCommand 兜底 */
    }
  }
  return execCommandCopy(text, container);
}

export function execCommandCopy(text: string, container?: HTMLElement): boolean {
  const host = container || document.body;
  const ta = document.createElement("textarea");
  ta.value = text;
  ta.setAttribute("readonly", "");
  ta.style.position = "absolute";
  ta.style.left = "-9999px";
  ta.style.top = "0";
  host.appendChild(ta);
  const sel = document.getSelection();
  const prev = sel && sel.rangeCount > 0 ? sel.getRangeAt(0) : null;
  ta.focus();
  ta.select();
  ta.setSelectionRange(0, text.length);
  let ok = false;
  try {
    ok = document.execCommand("copy");
  } catch {
    ok = false;
  }
  host.removeChild(ta);
  if (prev && sel) {
    sel.removeAllRanges();
    sel.addRange(prev);
  }
  return ok;
}
