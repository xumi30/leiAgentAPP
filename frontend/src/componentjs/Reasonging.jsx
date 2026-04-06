import { useState, useEffect, useRef, useCallback } from 'react';
import { GetReasoningMessage } from '../../wailsjs/go/main/App';
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime';

/**
 * @param {{ conversationId?: string }} props
 */
export default function Reasoning({ conversationId = '' }) {
  const [reasoningMessages, setReasoningMessages] = useState([]);
  const messagesRef = useRef(null);
  const [pinnedToBottom, setPinnedToBottom] = useState(true);

  const handleScroll = useCallback(() => {
    const el = messagesRef.current;
    if (!el) return;
    const thresholdPx = 24;
    const distanceToBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
    setPinnedToBottom(distanceToBottom <= thresholdPx);
  }, []);

  useEffect(() => {
    let cancelled = false;
    if (!conversationId) {
      setReasoningMessages([]);
      return () => {
        cancelled = true;
      };
    }
    (async () => {
      try {
        const rsm = await GetReasoningMessage(conversationId);
        if (!cancelled) {
          setReasoningMessages(Array.isArray(rsm) ? rsm : []);
        }
      } catch (e) {
        console.error('GetReasoningMessage:', e);
        if (!cancelled) setReasoningMessages([]);
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [conversationId]);

  useEffect(() => {
    const handleReasoningMessagAppend = (message) => {
      console.log('收到推理消息更新事件:', message);
      setReasoningMessages((prevMessages) => {
        if (!prevMessages || prevMessages.length === 0) {
          return [message];
        }
        const messageExists = prevMessages.some((msg) => msg.messageID === message.messageID);
        if (messageExists) {
          return prevMessages.map((msg) => {
            if (msg.messageID === message.messageID) {
              return {
                ...msg,
                content: msg.content + message.content,
              };
            }
            return msg;
          });
        }
        return [...prevMessages, message];
      });
    };

    EventsOn('reasoningAppend', handleReasoningMessagAppend);
    return () => {
      EventsOff('reasoningAppend');
    };
  }, []);

  useEffect(() => {
    const container = messagesRef.current;
    if (!container) return;

    if (!pinnedToBottom) return;
    const scrollId = requestAnimationFrame(() => {
      container.scrollTop = container.scrollHeight;
    });

    return () => cancelAnimationFrame(scrollId);
  }, [reasoningMessages, pinnedToBottom]);

  return (
    <div id={`reasoning_${conversationId}`} className="reasonings">
      <div className="reason__header">推理过程</div>
      <div className="reasoning__body">
        <div className="reasoning" ref={messagesRef} onScroll={handleScroll}>
          {reasoningMessages &&
            reasoningMessages.map((rsm) => (
              <div key={`reasoning_${rsm.messageID}`} className="reasoning__message">
                <span id={rsm.messageID} className="reasoning__message-text">
                  {rsm.content}
                </span>
              </div>
            ))}
        </div>
      </div>
    </div>
  );
}
