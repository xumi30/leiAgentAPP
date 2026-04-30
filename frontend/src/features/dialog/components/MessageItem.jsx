import React, { useMemo } from 'react';
import MessageContent from '../../../componentjs/MessageContent.jsx';
import userAvatar from '../../../assets/images/ren.png';
import assistantAvatar from '../../../assets/images/aitx.png';
import { isAssistantToolRoutineMessage } from '../../../utils/messageClassify';

function messageTokenCount(message) {
  const raw = message?.total_tokens ?? message?.totalTokens;
  const value = typeof raw === 'number' ? raw : Number.parseInt(String(raw ?? ''), 10);
  return Number.isFinite(value) && value > 0 ? value : 0;
}

/**
 * MessageItem
 * - 单条消息展示（UI only）
 *
 * @param {{
 *  msg: any,
 *  index: number,
 *  messages: any[],
 *  showAvatar: boolean,
 *  memoStripOpen: boolean,
 *  memoChecked: boolean,
 *  onToggleMemo: (messageId: string) => void,
 *  streamingHere: boolean,
 *  awaitingAssistantFirstChunk: boolean,
 *  agentsById: Map<string, any>,
 *  bodiesExpanded: boolean,
 *  onExpandAllBodies: () => void,
 *  onCollapseAllBodies: () => void,
 * }} props
 */
export default function MessageItem({
  msg,
  index,
  messages,
  showAvatar,
  memoStripOpen,
  memoChecked,
  onToggleMemo,
  streamingHere,
  awaitingAssistantFirstChunk,
  agentsById,
  bodiesExpanded,
  onExpandAllBodies,
  onCollapseAllBodies,
}) {
  const isUser = msg?.role === 'user';
  const mid = String(msg?.messageID ?? msg?.messageId ?? msg?.id ?? '');
  const tokenCount = messageTokenCount(msg);

  const toolRoutineCompact = useMemo(() => {
    if (isUser) return false;
    return isAssistantToolRoutineMessage(String(msg?.content ?? ''));
  }, [isUser, msg?.content]);

  const agentDisplay = useMemo(() => {
    const agentID = String(msg?.agentID ?? msg?.agent_id ?? msg?.agentId ?? '').trim();
    if (!agentID) return { imgSrc: assistantAvatar, label: '助手' };
    const agent = agentsById?.get?.(agentID);
    if (!agent) return { imgSrc: assistantAvatar, label: '助手' };
    const imgSrc = String(
      agent?.avatar_image
      ?? agent?.avatar
      ?? agent?.image_url
      ?? agent?.image
      ?? ''
    ).trim() || assistantAvatar;
    const label = String(agent?.agent_name ?? agent?.name ?? agent?.title ?? agentID).trim() || '助手';
    return { imgSrc, label };
  }, [agentsById, msg?.agentID, msg?.agent_id, msg?.agentId]);

  return (
    <div
      key={`dialogmessage_${mid || index}`}
      id={`dialogmessage_${mid || index}`}
      data-role={isUser ? 'user' : 'assistant'}
      className={
        'dialogmessage dialogmessage_' + (isUser ? 'user' : 'assistant')
        + (memoStripOpen ? ' dialogmessage--memo-pick' : '')
        + (!showAvatar ? ' dialogmessage--avatar-hidden' : '')
      }
    >
      {memoStripOpen ? (
        <label className="dialogmessage__memo-pick">
          <input
            type="checkbox"
            className="dialog__memo-checkbox-native"
            checked={memoChecked}
            onChange={() => onToggleMemo(mid)}
            aria-label="标记此条写入备忘录"
          />
        </label>
      ) : null}

      <div className="dialogmessage__body">
        {showAvatar ? (
          <div className="message-avatar clay-card">
            {isUser ? (
              <img src={userAvatar} alt="用户" className="message-avatar__img" />
            ) : (
              <>
                <img
                  src={agentDisplay.imgSrc}
                  onError={(e) => {
                    if (e.currentTarget?.src !== assistantAvatar) e.currentTarget.src = assistantAvatar;
                  }}
                  alt={agentDisplay.label}
                  className="message-avatar__img"
                />
                <span className="message-avatar__name">{agentDisplay.label}</span>
              </>
            )}
            <span className="message-timestamp">{msg?.timestamp}</span>
            {tokenCount > 0 ? (
              <span className="message-token-count" aria-label="此条消息 token 数">
                {tokenCount.toLocaleString()} tokens
              </span>
            ) : null}
          </div>
        ) : null}

        <div
          className={
            `messagecontent messagecontent--${isUser ? 'user' : 'assistant'}`
            + (streamingHere ? ' messagecontent--streaming' : '')
            + (awaitingAssistantFirstChunk ? ' messagecontent--user-awaiting' : '')
            + (toolRoutineCompact ? ' messagecontent--tool-routine' : '')
          }
        >
          {awaitingAssistantFirstChunk ? (
            <span
              className="message-user-awaiting-indicator"
              role="status"
              aria-live="polite"
              aria-label="等待回复"
            >
              <span className="message-user-awaiting-indicator__ring" aria-hidden />
            </span>
          ) : null}

          {streamingHere ? (
            <span
              className="message-streaming-indicator"
              role="status"
              aria-live="polite"
              aria-label="正在生成"
            >
              <span className="message-streaming-indicator__dot" aria-hidden />
            </span>
          ) : null}

          <MessageContent
            content={String(msg?.content ?? '')}
            variant={isUser ? 'user' : 'assistant'}
            isStreaming={Boolean(streamingHere)}
            bodiesExpanded={bodiesExpanded}
            onExpandAllBodies={onExpandAllBodies}
            onCollapseAllBodies={onCollapseAllBodies}
          />
        </div>
      </div>
    </div>
  );
}
