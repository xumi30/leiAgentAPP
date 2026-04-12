import React, { useState, useEffect, useLayoutEffect, useRef, useMemo, useCallback } from 'react';
import { EventsOff, EventsOn } from '../../wailsjs/runtime/runtime';
import {
    GetMessages,
    AppendMemoMarkdown,
    ComposeMemoWithLLM,
    GetMemoReferencedMessageIDs,
    SendMessage,
    SendUserDisplayOnly,
    StopChat,
} from '../../wailsjs/go/main/App';
import MessageContent from './MessageContent.jsx';
import {
    classifyUserMessage,
    classifyUserMessageLabel,
} from '../utils/messageClassify.js';

const MAIN_SHEET_ID = 'main';

/** 同 role 连续消息间隔 ≥ 此毫秒才再次显示头像 */
const MESSAGE_AVATAR_GROUP_GAP_MS = 3 * 60 * 1000;

const MEMO_CUSTOM_PRESETS_STORAGE_KEY = 'leiAgent.memoComposeCustomPresets.v1';

/** @param {unknown} ts */
function messageTimestampMs(ts) {
    if (ts == null || ts === '') return NaN;
    if (typeof ts === 'number') return Number.isFinite(ts) ? ts : NaN;
    const n = new Date(ts).getTime();
    return Number.isFinite(n) ? n : NaN;
}

/**
 * 与列表中上一条已展示消息相比：同 role 且发送间隔小于 3 分钟则不重复头像。
 * @param {{ role: string, timestamp?: unknown }[]} list
 * @param {number} index
 */
function shouldShowMessageAvatar(list, index) {
    if (index <= 0) return true;
    const cur = list[index];
    const prev = list[index - 1];
    if (!cur || !prev) return true;
    const curUser = cur.role === 'user';
    const prevUser = prev.role === 'user';
    if (curUser !== prevUser) return true;
    const curMs = messageTimestampMs(cur.timestamp);
    const prevMs = messageTimestampMs(prev.timestamp);
    if (Number.isNaN(curMs) || Number.isNaN(prevMs)) return true;
    return curMs - prevMs >= MESSAGE_AVATAR_GROUP_GAP_MS;
}

/** 内置快捷提示（不可删）；自定义项存 localStorage */
const MEMO_COMPOSE_PRESETS_DEFAULT = [
    { id: 'builtin:0', label: '傲娇女王', text: '以傲娇女王的口气总结一下' },
    { id: 'builtin:1', label: '萝莉语气', text: '以娇滴滴的萝莉语气复述一下' },
    { id: 'builtin:2', label: '项羽霸王', text: '以刚猛项羽霸王的姿态' },
    { id: 'builtin:3', label: '毛式智慧', text: '以毛主席的智慧讲讲' },
];

/** @returns {{ id: string, label: string, text: string }[]} */
function loadCustomMemoPresets() {
    try {
        const raw = localStorage.getItem(MEMO_CUSTOM_PRESETS_STORAGE_KEY);
        if (!raw) return [];
        const arr = JSON.parse(raw);
        if (!Array.isArray(arr)) return [];
        return arr
            .filter((p) => p && typeof p.label === 'string' && typeof p.text === 'string')
            .map((p) => ({
                id: typeof p.id === 'string' && p.id ? p.id : `u:${Date.now()}_${Math.random().toString(36).slice(2, 9)}`,
                label: p.label.trim().slice(0, 24),
                text: p.text.trim().slice(0, 800),
            }))
            .filter((p) => p.label && p.text);
    } catch {
        return [];
    }
}

/** @param {{ id: string, label: string, text: string }[]} list */
function saveCustomMemoPresets(list) {
    try {
        localStorage.setItem(MEMO_CUSTOM_PRESETS_STORAGE_KEY, JSON.stringify(list));
    } catch (e) {
        console.warn('saveCustomMemoPresets', e);
    }
}

function clipSheetTitle(text) {
    const line = String(text ?? '').split(/\r?\n/)[0].trim();
    if (!line) return '便签';
    const max = 22;
    return line.length > max ? `${line.slice(0, max)}…` : line;
}

/** 从助手正文取备忘录 # 标题（首行，去 Markdown 标题前缀） */
function titleForMemoFromBody(body) {
    const lines = String(body ?? '')
        .split(/\r?\n/)
        .map((l) => l.trim())
        .filter(Boolean);
    const first = lines[0] ?? '对话摘录';
    const t = first.replace(/^#{1,6}\s+/, '').slice(0, 56).trim();
    return t || '对话摘录';
}

/** @param {string} role */
function roleLabelForMemo(role) {
    if (role === 'user') return '用户';
    if (role === 'assistant') return '助手';
    return '消息';
}

/**
 * @param {{ role: string, content?: string }[]} orderedMarked 按时间顺序、已勾选的条目
 * @returns {{ title: string, body: string } | null}
 */
function buildMemoMarkdownFromMarked(orderedMarked) {
    if (!orderedMarked.length) return null;
    const parts = orderedMarked.map((m) => {
        const label = roleLabelForMemo(m.role);
        return `**${label}**\n\n${String(m.content ?? '').trim()}`;
    });
    const body = parts.join('\n\n---\n\n');
    const titleSource =
        orderedMarked.find((m) => m.role === 'user')?.content ??
        orderedMarked.find((m) => m.role !== 'user')?.content ??
        orderedMarked[0].content;
    const title = titleForMemoFromBody(titleSource).replace(/\s+/g, ' ').trim();
    return { title, body };
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
    /** 生成备忘：收窄消息区 + 按条勾选后写入 */
    const [memoStripOpen, setMemoStripOpen] = useState(false);
    /** @type {[Set<string>, React.Dispatch<React.SetStateAction<Set<string>>>]} */
    const [memoMarkedIds, setMemoMarkedIds] = useState(() => new Set());
    const [memoCheckSaving, setMemoCheckSaving] = useState(false);
    /** @type {[Set<string>, React.Dispatch<React.SetStateAction<Set<string>>>]} */
    const [memoRefIds, setMemoRefIds] = useState(() => new Set());
    const [memoComposeHint, setMemoComposeHint] = useState('');
    const [customMemoPresets, setCustomMemoPresets] = useState(loadCustomMemoPresets);
    const [memoPresetAddOpen, setMemoPresetAddOpen] = useState(false);
    const [memoPresetDraftLabel, setMemoPresetDraftLabel] = useState('');
    const [memoPresetDraftText, setMemoPresetDraftText] = useState('');
    const messagesRef = useRef(null);
    const [pinnedToBottom, setPinnedToBottom] = useState(true);
    const inputRef = useRef(null);
    /** 中文等 IME：拼音阶段为 true，避免回车被当成发送 */
    const imeComposingRef = useRef(false);
    const hintTimerRef = useRef(null);
    const chatIdRef = useRef(chatId);
    chatIdRef.current = chatId;

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

    const memoListMessages = useMemo(() => {
        const list = visibleMessages ?? [];
        return list.filter((msg) => {
            if (msg.role === 'reasoning') return false;
            const hasText = String(msg.content ?? '').trim() !== '';
            const streamingHere =
                streamInActiveSheet &&
                streamPulse &&
                String(streamPulse.chatID) === String(chatId) &&
                String(streamPulse.messageID) === String(msg.messageID);
            return hasText || streamingHere;
        });
    }, [visibleMessages, streamInActiveSheet, streamPulse, chatId]);

    const memoMarkedCount = memoMarkedIds.size;

    /** 当前便签内时间顺序上最后一条用户消息（用于「等待首包」时的 loading 锚点） */
    const lastUserMessageIdInSheet = useMemo(() => {
        const list = visibleMessages ?? [];
        for (let i = list.length - 1; i >= 0; i--) {
            if (list[i].role === 'user') return String(list[i].messageID);
        }
        return null;
    }, [visibleMessages]);

    const allMemoComposePresets = useMemo(
        () => [...MEMO_COMPOSE_PRESETS_DEFAULT, ...customMemoPresets],
        [customMemoPresets],
    );

    const addCustomMemoPreset = useCallback(() => {
        const label = memoPresetDraftLabel.trim().slice(0, 24);
        const text = memoPresetDraftText.trim().slice(0, 800);
        if (!label || !text) return;
        const id = `u:${Date.now()}_${Math.random().toString(36).slice(2, 9)}`;
        setCustomMemoPresets((prev) => {
            const next = [...prev, { id, label, text }];
            saveCustomMemoPresets(next);
            return next;
        });
        setMemoPresetDraftLabel('');
        setMemoPresetDraftText('');
        setMemoPresetAddOpen(false);
    }, [memoPresetDraftLabel, memoPresetDraftText]);

    const removeCustomMemoPreset = useCallback((id) => {
        setCustomMemoPresets((prev) => {
            const next = prev.filter((p) => p.id !== id);
            saveCustomMemoPresets(next);
            return next;
        });
    }, []);

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

    const refreshMemoRefIds = useCallback(async () => {
        try {
            const arr = await GetMemoReferencedMessageIDs();
            setMemoRefIds(new Set(Array.isArray(arr) ? arr : []));
        } catch (e) {
            console.error('GetMemoReferencedMessageIDs:', e);
        }
    }, []);

    useEffect(() => {
        if (memoStripOpen) {
            refreshMemoRefIds();
        } else {
            setMemoMarkedIds(new Set());
            setMemoComposeHint('');
            setMemoPresetAddOpen(false);
            setMemoPresetDraftLabel('');
            setMemoPresetDraftText('');
        }
    }, [memoStripOpen, refreshMemoRefIds]);

    const tryToggleMemoMark = useCallback((messageID) => {
        const id = String(messageID);
        setMemoMarkedIds((prev) => {
            const willSelect = !prev.has(id);
            if (willSelect && memoRefIds.has(id)) {
                if (!window.confirm('该消息曾写入过备忘录。是否仍加入本次摘录？')) {
                    return prev;
                }
            }
            const next = new Set(prev);
            if (next.has(id)) next.delete(id);
            else next.add(id);
            return next;
        });
    }, [memoRefIds]);

    /** 生成备忘展开时：点击消息列表背景（未点到某条消息）则收起生成备忘区 */
    const onMessagesMemoDismissMouseDown = useCallback(
        (e) => {
            if (!memoStripOpen || memoCheckSaving) return;
            if (e.target !== e.currentTarget) return;
            setMemoStripOpen(false);
        },
        [memoStripOpen, memoCheckSaving],
    );

    useEffect(() => {
        const handleConversationChange = (event) => {
            const { conversationId } = event.detail;
            setChatId(conversationId);
            setStreamPulse(null);
            setStopVisible(false);
            setPinnedToBottom(true);
            setSheets([{ id: MAIN_SHEET_ID, title: '主对话', startIdx: 0 }]);
            setActiveSheetId(MAIN_SHEET_ID);
            setClassifyHint('');
            setMemoStripOpen(false);
            setMemoMarkedIds(new Set());
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

    const handleMessagesScroll = useCallback(() => {
        const el = messagesRef.current;
        if (!el) return;
        const thresholdPx = 24;
        const distanceToBottom = el.scrollHeight - el.scrollTop - el.clientHeight;
        setPinnedToBottom(distanceToBottom <= thresholdPx);
    }, []);

    // 消息变化或流式输出时：仅当用户停留在底部时才自动滚到底部（layout 后执行，减少闪动）
    useLayoutEffect(() => {
        const container = messagesRef.current;
        if (!container) return;
        if (!pinnedToBottom) return;
        container.scrollTop = container.scrollHeight;
    }, [messages, streamPulse, visibleMessages, pinnedToBottom]);

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
            setStopVisible(false);
            setStreamPulse(null);
        }

        const handleDialogStreamEnd = (payload) => {
            const cid = String(payload?.chatID ?? '');
            const mid = String(payload?.messageID ?? '');
            setStreamPulse((prev) => {
                if (!prev) return prev;
                if (prev.chatID === cid && prev.messageID === mid) return null;
                return prev;
            });
            if (cid === chatIdRef.current) {
                setStopVisible(false);
            }
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
            // 暂时断开“新便签/页签”触发点：保留逻辑与数据结构，但不再创建/切换页签。
            // setSheets((prev) => [
            //     ...prev,
            //     { id: newId, title: clipSheetTitle(content), startIdx },
            // ]);
            // setActiveSheetId(newId);
            void newId;
            void startIdx;
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
        if (e.key !== 'Enter' || e.shiftKey) return;
        // 不要用 nativeEvent.isComposing：选字后按回车发送时，部分 WebView 仍会为 true，导致既不发送又拦了默认行为。
        // 仅用 composition 事件维护的 ref；229 表示 IME 正在处理该键（拼音阶段）。
        if (imeComposingRef.current) return;
        if (e.keyCode === 229 || e.which === 229) return;
        e.preventDefault();
        sendMessage();
    };

    const finishMemoAppend = useCallback(() => {
        window.dispatchEvent(
            new CustomEvent('memoSaved', { detail: { focusLatest: true } }),
        );
        setClassifyHint('已写入备忘录');
        if (hintTimerRef.current) clearTimeout(hintTimerRef.current);
        hintTimerRef.current = setTimeout(() => {
            setClassifyHint('');
            hintTimerRef.current = null;
        }, 2200);
        setMemoStripOpen(false);
        setMemoMarkedIds(new Set());
        setMemoComposeHint('');
        refreshMemoRefIds();
    }, [refreshMemoRefIds]);

    const formatMemoAppendBlock = (title, body, messageIds) => {
        const ids = messageIds.map(String).join(',');
        return `# ${title}\n\n${body}\n\n<!--leiAgent-memo-src:${ids}-->`;
    };

    const saveDirectMemo = useCallback(async () => {
        if (memoCheckSaving) return;
        const ordered = memoListMessages.filter((m) => memoMarkedIds.has(String(m.messageID)));
        if (ordered.length === 0) {
            alert('请先勾选要写入备忘录的消息。');
            return;
        }
        const built = buildMemoMarkdownFromMarked(ordered);
        if (!built) return;
        const ids = ordered.map((m) => m.messageID);
        setMemoCheckSaving(true);
        try {
            await AppendMemoMarkdown(formatMemoAppendBlock(built.title, built.body, ids));
            finishMemoAppend();
        } catch (err) {
            console.error('AppendMemoMarkdown:', err);
            alert(String(err?.message || err));
        } finally {
            setMemoCheckSaving(false);
        }
    }, [memoCheckSaving, memoListMessages, memoMarkedIds, finishMemoAppend]);

    const sendLLMMemo = useCallback(async () => {
        if (memoCheckSaving) return;
        const ordered = memoListMessages.filter((m) => memoMarkedIds.has(String(m.messageID)));
        if (ordered.length === 0) {
            alert('请先勾选要写入备忘录的消息。');
            return;
        }
        const built = buildMemoMarkdownFromMarked(ordered);
        if (!built) return;
        const draft = `## 摘录标题建议\n${built.title}\n\n## 对话摘录\n\n${built.body}`;
        const ids = ordered.map((m) => m.messageID);
        setMemoCheckSaving(true);
        try {
            const composed = await ComposeMemoWithLLM(draft, memoComposeHint);
            const block = `${String(composed).trim()}\n\n<!--leiAgent-memo-src:${ids.map(String).join(',')}-->`;
            await AppendMemoMarkdown(block);
            finishMemoAppend();
        } catch (err) {
            console.error('ComposeMemoWithLLM:', err);
            alert(String(err?.message || err));
        } finally {
            setMemoCheckSaving(false);
        }
    }, [memoCheckSaving, memoListMessages, memoMarkedIds, memoComposeHint, finishMemoAppend]);

    return (
        <div
            id={"dialog_" + chatId}
            className={'dialog' + (memoStripOpen ? ' dialog--memo-strip-open' : '')}
        >
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
                            // 暂时断开“点击页签切换”触发点：保留 UI，但不触发切换。
                            // onClick={() => setActiveSheetId(s.id)}
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
            <div
                className="dialog__messages"
                ref={messagesRef}
                onScroll={handleMessagesScroll}
                onMouseDown={onMessagesMemoDismissMouseDown}
            >
                {memoListMessages.map((msg, msgIndex) => {
                    const isUser = msg.role === 'user';
                    const showAvatar = shouldShowMessageAvatar(memoListMessages, msgIndex);
                    const streamingHere =
                        !isUser &&
                        streamInActiveSheet &&
                        streamPulse &&
                        String(streamPulse.chatID) === String(chatId) &&
                        String(streamPulse.messageID) === String(msg.messageID);
                    const awaitingAssistantFirstChunk =
                        isUser &&
                        stopVisible &&
                        !streamPulse &&
                        lastUserMessageIdInSheet != null &&
                        String(msg.messageID) === lastUserMessageIdInSheet;
                    const mid = String(msg.messageID);
                    return (
                        <div
                            key={'dialogmessage_' + msg.messageID}
                            id={'dialogmessage_' + msg.messageID}
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
                                        checked={memoMarkedIds.has(mid)}
                                        onChange={() => tryToggleMemoMark(mid)}
                                        aria-label="标记此条写入备忘录"
                                    />
                                </label>
                            ) : null}
                            <div className="dialogmessage__body">
                                {showAvatar ? (
                                    <div className="message-avatar clay-card">
                                        {isUser ? '🧑🏻' : '🤖'}
                                        <span className="message-timestamp">{msg.timestamp}</span>
                                    </div>
                                ) : null}
                                <div
                                    className={`messagecontent messagecontent--${isUser ? 'user' : 'assistant'}${streamingHere ? ' messagecontent--streaming' : ''}${awaitingAssistantFirstChunk ? ' messagecontent--user-awaiting' : ''}`}
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
                                        content={msg.content || ''}
                                        variant={isUser ? 'user' : 'assistant'}
                                        isStreaming={Boolean(streamingHere)}
                                    />
                                </div>
                            </div>
                        </div>
                    );
                })}
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
                            onCompositionStart={() => {
                                imeComposingRef.current = true;
                            }}
                            onCompositionEnd={() => {
                                imeComposingRef.current = false;
                            }}
                            onBlur={() => {
                                imeComposingRef.current = false;
                            }}
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
                <div className="dialog__memo-strip">
                    <div className="dialog__memo-toolbar" role="toolbar" aria-label="输入区快捷操作">
                        <button
                            type="button"
                            className="dialog__memo-pill"
                            disabled={memoCheckSaving}
                            onClick={() => {
                                if (memoCheckSaving) return;
                                setMemoStripOpen((v) => !v);
                            }}
                            aria-expanded={memoStripOpen}
                            title={
                                memoStripOpen
                                    ? '退出勾选模式，取消本次生成备忘'
                                    : '在消息旁勾选要收录的内容'
                            }
                        >
                            {memoStripOpen ? '取消生成' : '生成备忘'}
                        </button>
                    </div>
                    {memoStripOpen ? (
                        <div
                            className={`dialog__memo-compose${memoCheckSaving ? ' dialog__memo-compose--busy' : ''}`}
                        >
                            <p className="dialog__memo-compose-lead">
                                在每条消息旁勾选（可多选）。标题优先取<strong>用户</strong>消息首行，否则助手。
                            </p>
                            {memoMarkedCount > 0 ? (
                                <>
                                    <div
                                        className="dialog__memo-preset-bar"
                                        role="group"
                                        aria-label="快捷提示词，点击填入下方输入框"
                                    >
                                        <span className="dialog__memo-preset-bar__label">快捷</span>
                                        {allMemoComposePresets.map((p) => {
                                            const isBuiltin = String(p.id).startsWith('builtin:');
                                            return (
                                                <span key={p.id} className="dialog__memo-preset-chip-wrap">
                                                    <button
                                                        type="button"
                                                        className="dialog__memo-preset-chip"
                                                        disabled={memoCheckSaving}
                                                        title={p.text}
                                                        onClick={() => setMemoComposeHint(p.text)}
                                                    >
                                                        {p.label}
                                                    </button>
                                                    {!isBuiltin ? (
                                                        <button
                                                            type="button"
                                                            className="dialog__memo-preset-chip-del"
                                                            aria-label={`删除快捷「${p.label}」`}
                                                            disabled={memoCheckSaving}
                                                            onClick={(e) => {
                                                                e.preventDefault();
                                                                removeCustomMemoPreset(p.id);
                                                            }}
                                                        >
                                                            ×
                                                        </button>
                                                    ) : null}
                                                </span>
                                            );
                                        })}
                                        <button
                                            type="button"
                                            className="dialog__memo-preset-add"
                                            disabled={memoCheckSaving}
                                            title="添加自定义快捷提示词"
                                            aria-expanded={memoPresetAddOpen}
                                            aria-label="添加自定义快捷提示词"
                                            onClick={() =>
                                                setMemoPresetAddOpen((v) => {
                                                    if (v) {
                                                        setMemoPresetDraftLabel('');
                                                        setMemoPresetDraftText('');
                                                    }
                                                    return !v;
                                                })
                                            }
                                        >
                                            +
                                        </button>
                                    </div>
                                    {memoPresetAddOpen ? (
                                        <div className="dialog__memo-preset-editor">
                                            <div className="dialog__memo-preset-editor__row">
                                                <input
                                                    type="text"
                                                    className="dialog__memo-preset-editor__label"
                                                    placeholder="显示名称（短，如：严肃总结）"
                                                    value={memoPresetDraftLabel}
                                                    onChange={(e) =>
                                                        setMemoPresetDraftLabel(e.target.value)
                                                    }
                                                    maxLength={24}
                                                    autoComplete="off"
                                                />
                                                <textarea
                                                    className="dialog__memo-preset-editor__text"
                                                    rows={2}
                                                    placeholder="点击标签后填入的完整提示词…"
                                                    value={memoPresetDraftText}
                                                    onChange={(e) =>
                                                        setMemoPresetDraftText(e.target.value)
                                                    }
                                                    maxLength={800}
                                                />
                                            </div>
                                            <div className="dialog__memo-preset-editor__actions">
                                                <button
                                                    type="button"
                                                    className="dialog__memo-preset-editor__btn dialog__memo-preset-editor__btn--primary"
                                                    onClick={addCustomMemoPreset}
                                                >
                                                    添加
                                                </button>
                                                <button
                                                    type="button"
                                                    className="dialog__memo-preset-editor__btn"
                                                    onClick={() => {
                                                        setMemoPresetAddOpen(false);
                                                        setMemoPresetDraftLabel('');
                                                        setMemoPresetDraftText('');
                                                    }}
                                                >
                                                    取消
                                                </button>
                                            </div>
                                        </div>
                                    ) : null}
                                    <textarea
                                        className="dialog__memo-compose-input"
                                        rows={2}
                                        placeholder="可选：写给模型的整理要求（语气、侧重点、长度等）…"
                                        value={memoComposeHint}
                                        onChange={(e) => setMemoComposeHint(e.target.value)}
                                        disabled={memoCheckSaving}
                                    />
                                    <div className="dialog__memo-actions">
                                        <button
                                            type="button"
                                            className="dialog__memo-write-btn dialog__memo-write-btn--secondary"
                                            disabled={memoCheckSaving}
                                            onClick={saveDirectMemo}
                                        >
                                            直接写入
                                        </button>
                                        <button
                                            type="button"
                                            className="dialog__memo-write-btn"
                                            disabled={memoCheckSaving}
                                            onClick={sendLLMMemo}
                                        >
                                            {memoCheckSaving ? '处理中…' : '发送（模型优化）'}
                                        </button>
                                    </div>
                                </>
                            ) : null}
                        </div>
                    ) : null}
                </div>
            </div>
        </div>
    )
}
