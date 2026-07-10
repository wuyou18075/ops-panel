// @vitest-environment jsdom
import { describe, it, expect, vi } from "vitest";
import { copyToClipboard, execCommandCopy } from "./clipboard";

describe("copyToClipboard 安全上下文", () => {
  it("用异步 API 复制传入的原文", async () => {
    const writeText = vi.fn().mockResolvedValue(undefined);
    vi.stubGlobal("navigator", { clipboard: { writeText } });
    vi.stubGlobal("window", { isSecureContext: true });
    const ok = await copyToClipboard("curl xyz | sh");
    expect(ok).toBe(true);
    expect(writeText).toHaveBeenCalledWith("curl xyz | sh"); // 复制的正是传入命令
    vi.unstubAllGlobals();
  });

  it("空串返回 false", async () => {
    expect(await copyToClipboard("")).toBe(false);
  });
});

describe("execCommandCopy 兜底（非安全上下文）", () => {
  it("把原文放入容器内的 textarea 并复制该内容", () => {
    const container = document.createElement("div");
    document.body.appendChild(container);
    let copied = "";
    // jsdom 未实现 execCommand；打桩：复制时读取当前 textarea 的值，模拟真实复制行为
    (document as any).execCommand = vi.fn(() => {
      const ta = container.querySelector("textarea") as HTMLTextAreaElement | null;
      copied = ta ? ta.value : "";
      return true;
    });
    const ok = execCommandCopy("curl abc | sh", container);
    expect(ok).toBe(true);
    expect(copied).toBe("curl abc | sh"); // 复制内容 == 传入命令，且用完移除 textarea
    expect(container.querySelector("textarea")).toBeNull();
  });
});
