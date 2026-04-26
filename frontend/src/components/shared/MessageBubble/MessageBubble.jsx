import React from 'react';
import { useChatStore } from '../../../stores';
import { shouldShowMessageAvatar } from '../../../utils/messageClassify';
import userAvatar from '../../../assets/images/ren.png';
import assistantAvatar from '../../../assets/images/aitx.png';

const DEFAULT_ASSISTANT_AGENT_ID = 'agentid_0';

const MessageBubble = ({
  message,
  index,
  messages,
  isStreaming = false,
  // 移除 agentsById prop，直接从 store 获取
}) => {
  console.log(message);
  // 直接从 store 获取 agentsById
  const storeAgentsById = useChatStore((state) => state.agentsById);
  
  const isUser = message.role === 'user';
  const showAvatar = shouldShowMessageAvatar(index, messages);
  const timestampText = message.timestamp ? String(message.timestamp) : '';
  const agentIDRaw = String(message?.agentID ?? '').trim();
  
  const agentID = !isUser ? (agentIDRaw || DEFAULT_ASSISTANT_AGENT_ID) : '';

  const agent = agentID && storeAgentsById instanceof Map ? storeAgentsById.get(agentID) : null;
  const assistantAvatarSrc = String(agent?.avatar_image ?? '').trim() || assistantAvatar;
  const assistantLabel = String(agent?.agent_name ?? agentID).trim();
  alert(assistantAvatarSrc)
  return (
    <div
      data-role={isUser ? 'user' : 'assistant'}
      className={
        'dialogmessage dialogmessage_' +
        (isUser ? 'user' : 'assistant') +
        (!showAvatar ? ' dialogmessage--avatar-hidden' : '')
      }
    >
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
            {timestampText ? <span className="message-timestamp">{timestampText}</span> : null}
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

          <div className="message-body" style={{ whiteSpace: 'pre-wrap' }}>
            {String(message.content ?? '')}
          </div>
        </div>
      </div>
    </div>
  );
};

export default MessageBubble;