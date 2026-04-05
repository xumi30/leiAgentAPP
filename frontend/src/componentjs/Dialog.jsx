import React, { useState, useEffect, useLayoutEffect, useRef } from 'react';
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime';
import { GetMessages, SendMessage,StopChat } from '../../wailsjs/go/main/App';
import MessageContent from './MessageContent.jsx';

export default function Dialog() {
    // 获取对话列表
    const [chatId, setChatId] = useState('');
    const [messages, setMessages] = useState([]);
    const [stopVisible, setStopVisible] = useState(false);
    const [streamPulse, setStreamPulse] = useState(null);
    const messagesRef = useRef(null);
    const inputRef = useRef(null);

    useEffect(() => {
        const handleConversationChange = (event) => {
            const { conversationId } = event.detail;
            setChatId(conversationId);
            setStreamPulse(null);
            const getMessages = async () => {
                const messages = await GetMessages(conversationId);
                setMessages(messages);
                console.log("1收到消息更新事件:", messages);
            }
            getMessages();
        };

        window.addEventListener('conversationChanged', handleConversationChange);

        return () => {
            window.removeEventListener('conversationChanged', handleConversationChange);
        };
    }, []);

    // 消息变化或流式输出时，将列表滚到底部（layout 后执行，减少闪动）
    useLayoutEffect(() => {
        const container = messagesRef.current;
        if (!container) return;
        container.scrollTop = container.scrollHeight;
    }, [messages, streamPulse]);

    useEffect(() => {
        const handleMessage = (message) => {
            console.log("收到消息更新事件:", message);
            setMessages(prevMessages => {
                // 如果消息列表为空或不存在，直接返回包含新消息的数组
                if (!prevMessages?.length) {
                    return [message];
                }

                // 检查是否已存在该消息ID
                const messageExists = prevMessages.some(msg => msg.messageID === message.messageID);

                // 如果消息已存在，返回原数组；否则添加新消息
                return messageExists ? prevMessages : [...prevMessages, message];
            });

            setChatId(message.chatID); // 更新当前对话ID
        }

        const appendMessage = (message) => {
            console.log("收到消息更新事件:", message);
            setStreamPulse({
                chatID: String(message.chatID ?? ''),
                messageID: String(message.messageID ?? ''),
            });
            setMessages((prevMessages) => {
                if (!prevMessages || prevMessages.length === 0) {
                    return [message];
                }

                // 检查是否已存在该消息ID
                const messageExists = prevMessages.some(msg => msg.messageID === message.messageID);

                if (messageExists) {
                    // 使用 map 创建新数组，保持不可变性
                    return prevMessages.map((msg) => {
                        if (msg.messageID === message.messageID) {
                            // 创建新对象，保持不可变性
                            return {
                                ...msg,
                                content: msg.content + message.content
                            };
                        }
                        return msg;
                    });
                } else {
                    // 如果是新消息，添加到列表中
                    return [...prevMessages, message];
                }
            });

            setChatId(message.chatID); // 更新当前对话ID
        }

        const handleSenderror = (error) => {
            alert("发送消息失败: " + error);
            console.log("发送消息失败: ", error);
        }

        const handleDialogStreamEnd = (payload) => {
            const cid = String(payload?.chatID ?? '');
            const mid = String(payload?.messageID ?? '');
            setStreamPulse((prev) => {
                if (!prev) return prev;
                if (prev.chatID === cid && prev.messageID === mid) return null;
                return prev;
            });
        };

        EventsOn("dialogAppend", appendMessage); // 监听对话追加事件
        EventsOn("dialogStreamEnd", handleDialogStreamEnd);
        EventsOn("GetMessagesByMessageID", handleMessage); // 监听消息更新事件
        EventsOn("sendMessageError", handleSenderror); // 监听发送错误事件

        return () => {
            EventsOff("dialogAppend");
            EventsOff("dialogStreamEnd");
            EventsOff("GetMessagesByMessageID");
            EventsOff("sendMessageError");
        };
    }, []);



    const sendMessage = () => {
        const el = inputRef.current;
        if (!el) return;
        const content = el.value.trim();

        if (content) {
            SendMessage(chatId, content, "user");
            el.value = '';
            el.style.height = 'auto';
            setStopVisible(true);
        }
    };

    const stopDialog = () => {
        StopChat(chatId);
        setStopVisible(false);
        setStreamPulse(null);
    };

    /** @param {React.KeyboardEvent<HTMLTextAreaElement>} e */
    const onInputKeyDown = (e) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            sendMessage();
        }
    };

    return (
        <div id={"dialog_" + chatId} className="dialog">
            <div className="dialog__header">
                <span className="dialog__header-title">对话</span>
            </div>
            <div className="dialog__messages" ref={messagesRef}>
                {
                    messages && messages.filter((msg) => {
                        if (msg.role === 'reasoning') return false;
                        const hasText = String(msg.content ?? '').trim() !== '';
                        const streamingHere =
                            streamPulse &&
                            String(streamPulse.chatID) === String(chatId) &&
                            String(streamPulse.messageID) === String(msg.messageID);
                        return hasText || streamingHere;
                    }).map((msg) => {
                        const isUser = msg.role === 'user';
                        const streamingHere =
                            !isUser &&
                            streamPulse &&
                            String(streamPulse.chatID) === String(chatId) &&
                            String(streamPulse.messageID) === String(msg.messageID);
                        return (
                            <div key={"dialogmessage_" + msg.messageID}
                                id={"dialogmessage_" + msg.messageID}
                                data-role={isUser ? 'user' : 'assistant'}
                                className={`dialogmessage dialogmessage_${isUser ? 'user' : 'assistant'}`}>
                                <div className="message-avatar clay-card">
                                    {isUser ? '🧑🏻' : '🤖'}
                                    <span className="message-timestamp">
                                        {msg.timestamp}
                                    </span>
                                </div>
                                <div
                                    className={`messagecontent messagecontent--${isUser ? 'user' : 'assistant'}${streamingHere ? ' messagecontent--streaming' : ''}`}
                                >
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
                                        content={msg.content || ''}
                                        variant={isUser ? 'user' : 'assistant'}
                                        isStreaming={Boolean(streamingHere)}
                                    />
                                </div>
                            </div>
                        )
                    })
                }
            </div>

            <div className="dialog__input">
                <div className="dialog__input-row">
                    <button
                        type="button"
                        className={`dialog__btn-stop${stopVisible ? ' dialog__btn-stop--visible' : ''}`}
                        onClick={stopDialog}
                        aria-label="停止生成"
                        title="停止生成"
                    >
                        <span className="dialog__btn-stop-icon" aria-hidden>⏹</span>
                        <span className="dialog__btn-stop-text">停止</span>
                    </button>
                    <div className="dialog__textarea-shell">
                        <textarea
                            ref={inputRef}
                            className="dialog__textarea"
                            placeholder="输入消息，Enter 发送 · Shift+Enter 换行"
                            rows={1}
                            onKeyDown={onInputKeyDown}
                            onInput={(e) => {
                                const ta = e.target;
                                ta.style.height = 'auto';
                                ta.style.height = `${Math.min(ta.scrollHeight, 200)}px`;
                            }}
                        />
                    </div>
                    <button
                        type="button"
                        className="dialog__btn-send"
                        onClick={sendMessage}
                        aria-label="发送"
                        title="发送"
                    >
                        <span className="dialog__btn-send-icon" aria-hidden>🚀</span>
                    </button>
                </div>
            </div>
        </div>
    )
}
