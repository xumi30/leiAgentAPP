import { useCallback, useLayoutEffect, useState } from 'react';

/**
 * useDialogScroll
 * - 负责 pinnedToBottom 状态与“必要时自动滚到底部”
 *
 * @param {{ deps?: any[] }} opts
 * @returns {{
 *  pinnedToBottom: boolean,
 *  setPinnedToBottom: (v: boolean) => void,
 *  onScroll: () => void,
 *  scrollToBottomIfPinned: () => void,
 * }}
 */
export function useDialogScroll(opts = {}) {
  const { deps = [] } = opts;
  const [pinnedToBottom, setPinnedToBottom] = useState(true);

  const onScroll = useCallback((containerEl) => {
    const el = containerEl;
    if (!el) return;
    const thresholdPx = 24;
    const distanceToBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    setPinnedToBottom(distanceToBottom <= thresholdPx);
  }, []);

  const scrollToBottomIfPinned = useCallback((containerEl) => {
    const el = containerEl;
    if (!el) return;
    if (!pinnedToBottom) return;
    el.scrollTop = el.scrollHeight;
  }, [pinnedToBottom]);

  useLayoutEffect(() => {
    // 由调用方传入容器元素执行，避免 hook 直接持有 ref（便于复用）
    // 这里仅做“依赖变化时保持 pinned 状态”的语义占位。
    void deps;
  }, deps);

  return { pinnedToBottom, setPinnedToBottom, onScroll, scrollToBottomIfPinned };
}

