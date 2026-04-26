import { useEffect, useMemo } from 'react';
import { useStore, initStoreEventBridge } from '../../stores/useStore';

/**
 * useChatMessages（Dialog 维度）
 * - **事件监听与副作用**：全部收敛到 `useStore.initWailsEventBridge()`（只需初始化一次）
 * - **状态来源**：直接使用全局 store（避免 props drilling）
 *
 * @returns {{
 *  chatId: string,
 *  messages: any[],
 *  isStreaming: boolean,
 *  streamPulse: { chatID: string, messageID: string } | null,
 *  taskBusy: boolean,
 *  stopVisible: boolean,
 *  queuedInputs: { id: string, content: string }[],
 *  setStreamPulse: (p: any) => void,
 *  enqueueInput: (content: string) => void,
 *  dropQueuedInput: (id: string) => void,
 *  shiftQueuedInput: () => ({ id: string, content: string } | null),
 *  clearQueuedInputs: () => void,
 *  currentChatAgents: any[],
 *  allAgents: any[],
 *  memoStripOpen: boolean,
 *  setMemoStripOpen: (v: boolean) => void,
 * }}
 */
export function useChatMessages() {
  useEffect(() => initStoreEventBridge(), []);

  const chatId = useStore((s) => s.currentChatId);
  const messages = useStore((s) => s.messages);
  const isStreaming = useStore((s) => s.isStreaming);
  const streamPulse = useStore((s) => s.streamPulse);
  const taskBusy = useStore((s) => s.taskBusy);
  const stopVisible = useStore((s) => s.stopVisible);
  const queuedInputs = useStore((s) => s.queuedInputs);

  const setStreamPulse = useStore((s) => s.setStreamPulse);
  const enqueueInput = useStore((s) => s.enqueueInput);
  const dropQueuedInput = useStore((s) => s.dropQueuedInput);
  const shiftQueuedInput = useStore((s) => s.shiftQueuedInput);
  const clearQueuedInputs = useStore((s) => s.clearQueuedInputs);

  const currentChatAgents = useStore((s) => s.currentChatAgents);
  const allAgents = useStore((s) => s.allAgents);

  const memoStripOpen = useStore((s) => s.memoStripOpen);
  const setMemoStripOpen = useStore((s) => s.setMemoStripOpen);

  return useMemo(
    () => ({
      chatId,
      messages,
      isStreaming,
      streamPulse,
      taskBusy,
      stopVisible,
      queuedInputs,
      setStreamPulse,
      enqueueInput,
      dropQueuedInput,
      shiftQueuedInput,
      clearQueuedInputs,
      currentChatAgents,
      allAgents,
      memoStripOpen,
      setMemoStripOpen,
    }),
    [
      chatId,
      messages,
      isStreaming,
      streamPulse,
      taskBusy,
      stopVisible,
      queuedInputs,
      setStreamPulse,
      enqueueInput,
      dropQueuedInput,
      shiftQueuedInput,
      clearQueuedInputs,
      currentChatAgents,
      allAgents,
      memoStripOpen,
      setMemoStripOpen,
    ],
  );
}

