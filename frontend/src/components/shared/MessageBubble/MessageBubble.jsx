import React from 'react';
import { useChatStore } from '../../../stores';
import { shouldShowMessageAvatar } from '../../../utils/messageClassify';
import userAvatar from '../../../assets/images/ren.png';
import assistantAvatar from '../../../assets/images/aitx.png';
import MessageContent from '../../../componentjs/MessageContent.jsx';

const DEFAULT_ASSISTANT_AGENT_ID = 'agentid_0';
const formatBeijingTime = (timestamp) => {
  if (!timestamp) return '';
  const date = new Date(timestamp);
  // 使用toLocaleString方法，指定时区为Asia/Shanghai
  return date.toLocaleString('zh-CN', {
    timeZone: 'Asia/Shanghai',
    hour12: false,
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit'
  });
};
const MessageBubble = ({
  message,
  index,
  messages,
  isStreaming = false,
  memoStripOpen = false,
  memoChecked = false,
  onToggleMemo,
  // 移除 agentsById prop，直接从 store 获取
}) => {
  // 直接从 store 获取 agentsById
  const storeAgentsById = useChatStore((state) => state.agentsById);
  
  const isUser = message.role === 'user';
  const showAvatar = shouldShowMessageAvatar(index, messages);
  const mid = String(message?.messageID ?? message?.messageId ?? message?.id ?? '');
  const agentIDRaw = String(message?.agentID ?? '').trim();
  const timestampText1 = message.timestamp ? formatBeijingTime(message.timestamp) : '';
  const agentID = !isUser ? (agentIDRaw || DEFAULT_ASSISTANT_AGENT_ID) : '';

  const agent = agentID && storeAgentsById instanceof Map ? storeAgentsById.get(agentID) : null;
  const assistantAvatarSrc = String(agent?.avatar_image ?? '').trim() || assistantAvatar;
  const assistantLabel = String(agent?.agent_name ?? agentID).trim();
 
  return (
    <div
      data-role={isUser ? 'user' : 'assistant'}
      className={
        'dialogmessage dialogmessage_' +
        (isUser ? 'user' : 'assistant') +
        (memoStripOpen ? ' dialogmessage--memo-pick' : '') +
        (!showAvatar ? ' dialogmessage--avatar-hidden' : '')
      }
    >
      {memoStripOpen ? (
        <label className="dialogmessage__memo-pick">
          <input
            type="checkbox"
            className="dialog__memo-checkbox-native"
            checked={Boolean(memoChecked)}
            onChange={() => onToggleMemo?.(mid)}
            aria-label="标记此条写入备忘录"
          />
        </label>
      ) : null}
      <div className="dialogmessage__body">
        {showAvatar ? (
          <div className="message-avatar clay-card">
            <img
              src={isUser ? userAvatar : assistantAvatarSrc}
              onError={(e) => {
                if (isUser) {
                  if (e.currentTarget?.src !== userAvatar) e.currentTarget.src = userAvatar;
                  return;
                }
                if (e.currentTarget?.src !== assistantAvatar) e.currentTarget.src = assistantAvatar;
              }}
              alt={isUser ? '用户' : assistantLabel || '助手'}
              className="message-avatar__img"
            />
            {!isUser && assistantLabel ? (
              <span className="message-avatar__name">{assistantLabel}</span>
            ) : null}
            {timestampText1 ? <span className="message-timestamp">{timestampText1}</span> : null}
          </div>
        ) : null}

        <div
          className={`messagecontent messagecontent--${isUser ? 'user' : 'assistant'}${!isUser && isStreaming ? ' messagecontent--streaming' : ''}`}
        >
          {!isUser && isStreaming ? (
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
            content={String(message.content ?? '')}
            variant={isUser ? 'user' : 'assistant'}
            isStreaming={Boolean(!isUser && isStreaming)}
          />
        </div>
      </div>
    </div>
  );
};

export default MessageBubble;