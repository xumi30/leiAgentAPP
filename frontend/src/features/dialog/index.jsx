import React, { useCallback, useMemo, useRef, useState } from 'react';
import { SendMessage, SendUserDisplayOnly, StopChat } from '../../../wailsjs/go/main/App';
import { classifyUserMessage, classifyUserMessageLabel } from '../../utils/messageClassify.js';
import { getRandomMacaronColor } from '../../componentjs/Constant';
import '../../componentcss/Dialog.css';

import { MAIN_SHEET_ID, TRANSIENT_HINT_MS } from './constants';
import { useChatMessages } from './hooks/useChatMessages';
import { useDialogScroll } from './hooks/useDialogScroll';
import { useMemoComposer } from './hooks/useMemoComposer';

import TabHeader from './components/TabHeader';
import MessageList from './components/MessageList';
import ChatInput from './components/ChatInput';
import MemoStrip from './components/MemoStrip';

const DEFAULT_ASSISTANT_AGENT_ID = 'agentid_0';

function toStringSafe(v) {
  return String(v ?? '');
}

export default function Dialog() {
  const {
    chatId,
    messages,
    isStreaming,
    streamPulse,
    setStreamPulse,
    taskBusy,
    stopVisible,
    queuedInputs,
    enqueueInput,
    dropQueuedInput,
    shiftQueuedInput,
    clearQueuedInputs,
    currentChatAgents,
    allAgents,
    memoStripOpen,
    setMemoStripOpen,
  } = useChatMessages();

  const chatIdRef = useRef(chatId);
  chatIdRef.current = chatId;
  const taskBusyRef = useRef(taskBusy);
  taskBusyRef.current = taskBusy;

  const [conversationTitle, setConversationTitle] = useState('');
  const [activeSheetId, setActiveSheetId] = useState(MAIN_SHEET_ID);
  const [sheets] = useState([{ id: MAIN_SHEET_ID, title: '主对话', startIdx: 0 }]);

  const [classifyHint, setClassifyHint] = useState('');
  const [runtimeError, setRuntimeError] = useState('');
  const hintTimerRef = useRef(null);

  const [inputValue, setInputValue] = useState('');
  const [aiteAgentIds, setAiteAgentIds] = useState(() => new Set());

  const [allMessageBodiesExpanded, setAllMessageBodiesExpanded] = useState(false);
  const expandAllMessageBodies = useCallback(() => setAllMessageBodiesExpanded(true), []);
  const collapseAllMessageBodies = useCallback(() => setAllMessageBodiesExpanded(false), []);

  const currentChatAgentsForUI = useMemo(() => {
    const list = Array.isArray(currentChatAgents) ? [...currentChatAgents] : [];
    const hasDefault = list.some(
      (a) => String(a?.agentID ?? a?.agent_id ?? '').trim() === DEFAULT_ASSISTANT_AGENT_ID,
    );
    if (hasDefault) return list;

    const fromAll = (Array.isArray(allAgents) ? allAgents : []).find(
      (a) => String(a?.agent_id ?? a?.agentID ?? '').trim() === DEFAULT_ASSISTANT_AGENT_ID,
    );
    if (fromAll) return [fromAll, ...list];
    return [
      {
        agentID: DEFAULT_ASSISTANT_AGENT_ID,
        agent_id: DEFAULT_ASSISTANT_AGENT_ID,
        agent_name: '工具人',
        avatar_image: '',
      },
      ...list,
    ];
  }, [currentChatAgents, allAgents]);

  const conversationAgentOptions = useMemo(() => {
    return currentChatAgentsForUI
      .map((a) => ({
        agent_id: String(a?.agentID ?? a?.agent_id ?? '').trim(),
        agent_name: String(a?.agent_name ?? '').trim(),
        avatar_image: String(a?.avatar_image ?? '').trim(),
      }))
      .filter((a) => a.agent_id && a.agent_name);
  }, [currentChatAgentsForUI]);

  const agentsById = useMemo(() => {
    const m = new Map();
    const inChat = Array.isArray(currentChatAgents) ? currentChatAgents : [];
    const all = Array.isArray(allAgents) ? allAgents : [];
    for (const a of inChat) {
      const id = String(a?.agentID ?? a?.agent_id ?? a?.agentId ?? '').trim();
      if (id) m.set(id, a);
    }
    for (const a of all) {
      const id = String(a?.agentID ?? a?.agent_id ?? a?.agentId ?? '').trim();
      if (id) m.set(id, a);
    }
    if (!m.has(DEFAULT_ASSISTANT_AGENT_ID)) {
      m.set(DEFAULT_ASSISTANT_AGENT_ID, {
        agentID: DEFAULT_ASSISTANT_AGENT_ID,
        agent_name: '工具人',
        avatar_image: '',
      });
    }
    return m;
  }, [currentChatAgents, allAgents]);

  const sortedSheets = useMemo(() => [...sheets].sort((a, b) => a.startIdx - b.startIdx), [sheets]);
  const activeSheet = useMemo(
    () => sortedSheets.find((s) => s.id === activeSheetId) ?? sortedSheets[0],
    [sortedSheets, activeSheetId],
  );

  const visibleMessages = useMemo(() => {
    const sh = activeSheet;
    const all = Array.isArray(messages) ? messages : [];
    if (!sh) return all;
    const si = sortedSheets.findIndex((s) => s.id === sh.id);
    const start = sh.startIdx;
    const end = si >= 0 && si < sortedSheets.length - 1 ? sortedSheets[si + 1].startIdx : all.length;
    return all.slice(start, end);
  }, [messages, activeSheet, sortedSheets]);

  const memoListMessages = useMemo(() => {
    const list = Array.isArray(visibleMessages) ? visibleMessages : [];
    return list.filter((msg) => {
      if (msg?.role === 'reasoning') return false;
      const hasText = String(msg?.content ?? '').trim() !== '';
      const streamingHere =
        Boolean(streamPulse)
        && String(streamPulse?.chatID ?? '') === String(chatId ?? '')
        && String(streamPulse?.messageID ?? '') === String(msg?.messageID ?? '');
      return hasText || streamingHere;
    });
  }, [visibleMessages, streamPulse, chatId]);

  const stopButtonEngaged =
    stopVisible
    || (
      Boolean(streamPulse)
      && String(streamPulse?.chatID ?? '') === String(chatId ?? '')
    );

  const showTransientHint = useCallback((text, ms = TRANSIENT_HINT_MS) => {
    setClassifyHint(text);
    if (hintTimerRef.current) window.clearTimeout(hintTimerRef.current);
    hintTimerRef.current = window.setTimeout(() => {
      setClassifyHint('');
      hintTimerRef.current = null;
    }, ms);
  }, []);

  const showRuntimeError = useCallback((text, ms = 6000) => {
    const msg = String(text ?? '').trim();
    if (!msg) return;
    setRuntimeError(msg);
    if (hintTimerRef.current) window.clearTimeout(hintTimerRef.current);
    hintTimerRef.current = window.setTimeout(() => {
      setRuntimeError('');
      hintTimerRef.current = null;
    }, ms);
  }, []);

  const showClassifyHint = useCallback((kind) => {
    const label = classifyUserMessageLabel(kind);
    showTransientHint(`已归类为「${label}」`, TRANSIENT_HINT_MS);
  }, [showTransientHint]);

  const dispatchUserMessage = useCallback((targetChatId, content) => {
    const text = String(content ?? '').trim();
    if (!text) return;
    SendMessage(targetChatId, JSON.stringify({ content: text, aite: Array.from(aiteAgentIds) }), 'user');
  }, [aiteAgentIds]);

  const sendMessage = useCallback(async () => {
    const content = String(inputValue ?? '').trim();
    if (!content) return;

    const streaming = Boolean(streamPulse) || Boolean(isStreaming);
    const kind = classifyUserMessage(content, { isStreaming: streaming });
    showClassifyHint(kind);

    if (kind === 'control') {
      try {
        await SendUserDisplayOnly(chatId, content);
      } catch (e) {
        console.error('SendUserDisplayOnly:', e);
      }
      StopChat(chatId);
      setStreamPulse(null);
      setInputValue('');
      setAiteAgentIds(new Set());
      return;
    }

    if (taskBusyRef.current) {
      enqueueInput(content);
      showTransientHint(`已缓存到待发送队列（${queuedInputs.length + 1}）`);
      setInputValue('');
      setAiteAgentIds(new Set());
      return;
    }

    dispatchUserMessage(chatId, content);
    setInputValue('');
    setAiteAgentIds(new Set());
  }, [
    inputValue,
    streamPulse,
    isStreaming,
    chatId,
    setStreamPulse,
    enqueueInput,
    queuedInputs.length,
    dispatchUserMessage,
    showClassifyHint,
    showTransientHint,
  ]);

  const sendQueuedInput = useCallback((id) => {
    if (taskBusyRef.current) return;
    const picked = shiftQueuedInput();
    if (picked?.id !== id) {
      if (picked?.content) enqueueInput(picked.content);
    }
    if (picked?.content) {
      dispatchUserMessage(chatIdRef.current, picked.content);
      setAiteAgentIds(new Set());
    }
  }, [shiftQueuedInput, enqueueInput, dispatchUserMessage]);

  const stopDialog = useCallback(() => {
    StopChat(chatId);
    setStreamPulse(null);
  }, [chatId, setStreamPulse]);

  const onMessagesMemoDismissMouseDown = useCallback((e, memoCheckSaving) => {
    if (!memoStripOpen || memoCheckSaving) return;
    if (e.target !== e.currentTarget) return;
    setMemoStripOpen(false);
  }, [memoStripOpen, setMemoStripOpen]);

  const { pinnedToBottom, onScroll, scrollToBottomIfPinned } = useDialogScroll();
  const messagesContainerRef = useRef(null);
  const handleScroll = useCallback((el) => onScroll(el), [onScroll]);

  const memoComposer = useMemoComposer({
    open: memoStripOpen,
    messages: memoListMessages,
    onClose: () => setMemoStripOpen(false),
    onHint: (t) => showTransientHint(t),
    onError: (t) => showRuntimeError(t, 2600),
  });

  const listMacaron = useMemo(() => getRandomMacaronColor(String(chatId ?? '')), [chatId]);
  const conversationTokenTotal = useMemo(() => {
    let sum = 0;
    for (const msg of messages ?? []) {
      const raw = msg?.total_tokens ?? msg?.totalTokens;
      const value = typeof raw === 'number' ? raw : Number.parseInt(toStringSafe(raw), 10);
      if (Number.isFinite(value) && value > 0) sum += value;
    }
    return sum;
  }, [messages]);

  return (
    <div
      id={`dialog_${String(chatId ?? '')}`}
      className={'dialog' + (memoStripOpen ? ' dialog--memo-strip-open' : '')}
    >
      <TabHeader
        sortedSheets={sortedSheets}
        activeSheetId={activeSheetId}
        conversationTitle={conversationTitle}
        conversationTokenTotal={conversationTokenTotal}
        listMacaron={listMacaron}
        currentChatAgents={currentChatAgentsForUI}
      />

      {classifyHint ? (
        <div className="dialog__classify-hint" role="status">{classifyHint}</div>
      ) : null}
      {runtimeError ? (
        <div className="dialog__classify-hint" role="alert">{runtimeError}</div>
      ) : null}

      <div ref={messagesContainerRef}>
        <MessageList
          chatId={chatId}
          messages={memoListMessages}
          streamPulse={streamPulse}
          stopVisible={stopVisible}
          memoStripOpen={memoStripOpen}
          memoMarkedIds={memoComposer.memoMarkedIds}
          onToggleMemo={memoComposer.tryToggleMemoMark}
          onScroll={(el) => {
            handleScroll(el);
            scrollToBottomIfPinned(el);
          }}
          onBackgroundMouseDown={(e) => onMessagesMemoDismissMouseDown(e, memoComposer.memoCheckSaving)}
          agentsById={agentsById}
          bodiesExpanded={allMessageBodiesExpanded}
          onExpandAllBodies={expandAllMessageBodies}
          onCollapseAllBodies={collapseAllMessageBodies}
        />
      </div>

      <div className="dialog__input">
        {queuedInputs.length > 0 ? (
          <div className="dialog__queued-strip" role="status" aria-live="polite">
            <div className="dialog__queued-strip-head">
              <span className="dialog__queued-strip-title">{queuedInputs.length} Queued</span>
              <button
                type="button"
                className="dialog__queued-strip-clear"
                onClick={() => clearQueuedInputs()}
                title="清空队列"
              >
                清空
              </button>
            </div>
            <div className="dialog__queued-list">
              {queuedInputs.map((item, idx) => (
                <div key={item.id} className="dialog__queued-item">
                  <span className="dialog__queued-item-bullet" aria-hidden />
                  <span className="dialog__queued-item-text">{item.content}</span>
                  <div className="dialog__queued-item-actions">
                    <button
                      type="button"
                      className="dialog__queued-item-btn"
                      onClick={() => sendQueuedInput(item.id)}
                      aria-label={`发送待发送消息 ${idx + 1}`}
                      title={taskBusy ? '当前仍在处理中' : '发送这条'}
                      disabled={taskBusy}
                    >
                      ↑
                    </button>
                    <button
                      type="button"
                      className="dialog__queued-item-btn"
                      onClick={() => dropQueuedInput(item.id)}
                      aria-label={`移除待发送消息 ${idx + 1}`}
                      title="移除"
                    >
                      🗑
                    </button>
                  </div>
                </div>
              ))}
            </div>
          </div>
        ) : null}

        <div className="dialog__input-row">
          <button
            type="button"
            className="dialog__btn-stop dialog__btn-stop--visible"
            onClick={stopDialog}
            disabled={!stopButtonEngaged}
          >
            <span className="dialog__btn-stop-icon" aria-hidden>⏹</span>
            <span className="dialog__btn-stop-text">{stopButtonEngaged ? '跑马中' : '摸鱼中'}</span>
          </button>

          <ChatInput
            value={inputValue}
            onChange={(v) => {
              setInputValue(v);
              if (!String(v ?? '').trim()) setAiteAgentIds(new Set());
            }}
            onSend={() => void sendMessage()}
            mentionOptions={conversationAgentOptions}
            onMentionPicked={(picked) => {
              const id = String(picked?.agent_id ?? picked?.agentID ?? '').trim();
              if (!id) return;
              setAiteAgentIds((prev) => {
                const next = new Set(prev);
                next.add(id);
                return next;
              });
            }}
            disabled={false}
          />

          <button
            type="button"
            className="dialog__btn-send"
            onClick={() => void sendMessage()}
            aria-label="发送"
            title="发送"
          >
            <span className="dialog__btn-send-icon" aria-hidden>🚀</span>
          </button>
        </div>

        <MemoStrip
          open={memoStripOpen}
          busy={memoComposer.memoCheckSaving}
          markedCount={memoComposer.memoMarkedCount}
          presets={memoComposer.allMemoComposePresets}
          presetAddOpen={memoComposer.memoPresetAddOpen}
          draftLabel={memoComposer.memoPresetDraftLabel}
          draftText={memoComposer.memoPresetDraftText}
          composeHint={memoComposer.memoComposeHint}
          onToggleOpen={() => {
            if (memoComposer.memoCheckSaving) return;
            setMemoStripOpen((v) => !v);
          }}
          onSetComposeHint={memoComposer.setMemoComposeHint}
          onSaveDirect={() => void memoComposer.saveDirectMemo()}
          onSendLLM={() => void memoComposer.sendLLMMemo()}
          onTogglePresetAdd={() => memoComposer.setMemoPresetAddOpen((v) => !v)}
          onDraftLabel={memoComposer.setMemoPresetDraftLabel}
          onDraftText={memoComposer.setMemoPresetDraftText}
          onAddPreset={memoComposer.addCustomMemoPreset}
          onRemovePreset={memoComposer.removeCustomMemoPreset}
        />
      </div>
    </div>
  );
}

