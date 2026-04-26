import React, { useEffect, useMemo, useRef } from 'react';
import MessageItem from './MessageItem';
import { shouldShowMessageAvatar } from '../utils/messageHelpers';

/**
 * MessageList
 * - 消息列表容器（UI only）
 *
 * @param {{
 *  chatId: string,
 *  messages: any[],
 *  streamPulse: { chatID: string, messageID: string } | null,
 *  stopVisible: boolean,
 *  memoStripOpen: boolean,
 *  memoMarkedIds: Set<string>,
 *  onToggleMemo: (messageId: string) => void,
 *  onScroll: (el: HTMLDivElement) => void,
 *  onBackgroundMouseDown?: (e: any) => void,
 *  agentsById: Map<string, any>,
 *  bodiesExpanded: boolean,
 *  onExpandAllBodies: () => void,
 *  onCollapseAllBodies: () => void,
 * }} props
 */
export default function MessageList({
  chatId,
  messages,
  streamPulse,
  stopVisible,
  memoStripOpen,
  memoMarkedIds,
  onToggleMemo,
  onScroll,
  onBackgroundMouseDown,
  agentsById,
  bodiesExpanded,
  onExpandAllBodies,
  onCollapseAllBodies,
}) {
  const messagesRef = useRef(null);

  const list = Array.isArray(messages) ? messages : [];

  const lastUserMessageId = useMemo(() => {
    for (let i = list.length - 1; i >= 0; i--) {
      if (list[i]?.role === 'user') return String(list[i]?.messageID ?? '');
    }
    return null;
  }, [list]);

  useEffect(() => {
    const el = messagesRef.current;
    if (!el) return;
    // 初始化 pinned 判定：交给上层 hook 决定是否要滚动
    onScroll?.(el);
  }, [onScroll]);

  return (
    <div
      className="dialog__messages"
      ref={messagesRef}
      onScroll={() => onScroll?.(messagesRef.current)}
      onMouseDown={onBackgroundMouseDown}
    >
      {list.map((msg, idx) => {
        const isUser = msg?.role === 'user';
        const mid = String(msg?.messageID ?? msg?.messageId ?? msg?.id ?? '');

        const showAvatar = shouldShowMessageAvatar(list, idx);

        const streamingHere = !isUser
          && Boolean(streamPulse)
          && String(streamPulse?.chatID ?? '') === String(chatId ?? '')
          && String(streamPulse?.messageID ?? '') === String(msg?.messageID ?? '');

        const awaitingAssistantFirstChunk =
          isUser
          && stopVisible
          && !streamPulse
          && lastUserMessageId != null
          && String(mid) === String(lastUserMessageId);

        return (
          <MessageItem
            key={`m_${mid || idx}`}
            msg={msg}
            index={idx}
            messages={list}
            showAvatar={showAvatar}
            memoStripOpen={memoStripOpen}
            memoChecked={memoMarkedIds?.has?.(String(mid))}
            onToggleMemo={onToggleMemo}
            streamingHere={streamingHere}
            awaitingAssistantFirstChunk={awaitingAssistantFirstChunk}
            agentsById={agentsById}
            bodiesExpanded={bodiesExpanded}
            onExpandAllBodies={onExpandAllBodies}
            onCollapseAllBodies={onCollapseAllBodies}
          />
        );
      })}
    </div>
  );
}

