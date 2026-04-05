import React, { useState, useEffect, useLayoutEffect, useRef, useMemo, useCallback } from 'react';
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime';
import { GetMessages, SendMessage, SendUserDisplayOnly, StopChat } from '../../wailsjs/go/main/App';
import MessageContent from './MessageContent.jsx';
import {
    classifyUserMessage,
    classifyUserMessageLabel,
} from '../utils/messageClassify.js';

const MAIN_SHEET_ID = 'main';

function clipSheetTitle(text) {
    const line = String(text ?? '').split(/\r?\n/)[0].trim();
    if (!line) return '便签';
    const max = 22;
    return line.length > max ? `${line.slice(0, max)}…` : line;
}

export default function Dialog() {
    // 获取对话列表
    const [chatId, setChatId] = useState('');
    const [messages, setMessages] = useState([]);
    const [stopVisible, setStopVisible] = useState(false);
    const [streamPulse, setStreamPulse] = useState(null);
    /** @type {[{ id: string, title: string, startIdx: number }]} */
    const [sheets, setSheets] = useState([
        { id: MAIN_SHEET_ID, title: '主对话', startIdx: 0 },
    ]);
    const [activeSheetId, setActiveSheetId] = useState(MAIN_SHEET_ID);
    const [classifyHint, setClassifyHint] = useState('');
    const messagesRef = useRef(null);
    const inputRef = useRef(null);
    const hintTimerRef = useRef(null);

    const sortedSheets = useMemo(
        () => [...sheets].sort((a, b) => a.startIdx - b.startIdx),
        [sheets],
    );

    const activeSheet = useMemo(
        () => sortedSheets.find((s) => s.id === activeSheetId) ?? sortedSheets[0],
        [sortedSheets, activeSheetId],
    );

    const { visibleMessages, streamInActiveSheet } = useMemo(() => {
        const sh = activeSheet;
        const all = messages ?? [];
        let idx = -1;
        if (streamPulse) {
            idx = all.findIndex(
                (m) => String(m.messageID) === String(streamPulse.messageID),
            );
        }
        if (!sh) {
            return {
                visibleMessages: all,
                streamInActiveSheet: false,
            };
        }
        const si = sortedSheets.findIndex((s) => s.id === sh.id);
        const start = sh.startIdx;
        const end =
            si >= 0 && si < sortedSheets.length - 1
                ? sortedSheets[si + 1].startIdx
                : all.length;
        const slice = all.slice(start, end);
        const inSheet = idx >= start && idx < end;
        return {
            visibleMessages: slice,
            streamInActiveSheet: inSheet,
        };
    }, [messages, activeSheet, sortedSheets, streamPulse]);

    const showClassifyHint = useCallback((kind) => {
        const label = classifyUserMessageLabel(kind);
        setClassifyHint(`已归类为「${label}」`);
        if (hintTimerRef.current) clearTimeout(hintTimerRef.current);
        hintTimerRef.current = setTimeout(() => {
            setClassifyHint('');
            hintTimerRef.current = null;
        }, 2200);
    }, []);

    useEffect(() => {
        return () => {
            if (hintTimerRef.current) clearTimeout(hintTimerRef.current);
        };
    }, []);

    useEffect(() => {
        const handleConversationChange = (event) => {
            const { conversationId } = event.detail;
            setChatId(conversationId);
            setStreamPulse(null);
            setSheets([{ id: MAIN_SHEET_ID, title: '主对话', startIdx: 0 }]);
            setActiveSheetId(MAIN_SHEET_ID);
            setClassifyHint('');
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
    }, [messages, streamPulse, visibleMessages]);

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
                    // 使用 map 创建新数组，保持不可变性；timestamp 以首包为准
                    return prevMessages.map((msg) => {
                        if (msg.messageID === message.messageID) {
                            // 创建新对象，保持不可变性
                            return {
                                ...msg,
                                content: msg.content + message.content,
                            };
                        }
                        return msg;
                    });
                } else {
                    // 新消息：携带后端首包时间，与 DB 排序一致
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



    const sendMessage = async () => {
        const el = inputRef.current;
        if (!el) return;
        const content = el.value.trim();

        if (!content) return;

        const streaming = Boolean(streamPulse);
        const kind = classifyUserMessage(content, { isStreaming: streaming });
        showClassifyHint(kind);

        if (kind === 'control') {
            try {
                await SendUserDisplayOnly(chatId, content);
            } catch (e) {
                console.error('SendUserDisplayOnly:', e);
            }
            StopChat(chatId);
            setStopVisible(false);
            setStreamPulse(null);
            el.value = '';
            el.style.height = 'auto';
            return;
        }

        if (kind === 'newTopic') {
            const newId = `sheet_${Date.now()}_${Math.random().toString(36).slice(2, 9)}`;
            const startIdx = messages.length;
            setSheets((prev) => [
                ...prev,
                { id: newId, title: clipSheetTitle(content), startIdx },
            ]);
            setActiveSheetId(newId);
        }

        SendMessage(chatId, content, "user");
        el.value = '';
        el.style.height = 'auto';
        setStopVisible(true);
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
            <div className="dialog__header dialog__header--tabs">
                <div
                    className="dialog__tabs"
                    role="tablist"
                    aria-label="同一会话便签页"
                >
                    {sortedSheets.map((s) => (
                        <button
                            key={s.id}
                            type="button"
                            role="tab"
                            aria-selected={activeSheetId === s.id}
                            className={
                                'dialog__tab' +
                                (activeSheetId === s.id ? ' dialog__tab--active' : '')
                            }
                            onClick={() => setActiveSheetId(s.id)}
                            title={s.title}
                        >
                            <span className="dialog__tab-label">{s.title}</span>
                        </button>
                    ))}
                </div>
            </div>
            {classifyHint ? (
                <div className="dialog__classify-hint" role="status">
                    {classifyHint}
                </div>
            ) : null}
            <div className="dialog__messages" ref={messagesRef}>
                {
                    visibleMessages && visibleMessages.filter((msg) => {
                        if (msg.role === 'reasoning') return false;
                        const hasText = String(msg.content ?? '').trim() !== '';
                        const streamingHere =
                            streamInActiveSheet &&
                            streamPulse &&
                            String(streamPulse.chatID) === String(chatId) &&
                            String(streamPulse.messageID) === String(msg.messageID);
                        return hasText || streamingHere;
                    }).map((msg) => {
                        const isUser = msg.role === 'user';
                        const streamingHere =
                            !isUser &&
                            streamInActiveSheet &&
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
