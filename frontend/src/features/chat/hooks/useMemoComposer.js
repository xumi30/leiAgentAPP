import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import {
  AppendMemoMarkdown,
  ComposeMemoWithLLM,
  ConfirmMemoReinclude,
  GetMemoReferencedMessageIDs,
} from '../../../../wailsjs/go/main/App';
import {
  buildMemoMarkdownFromMarked,
  formatMemoAppendBlock,
  loadCustomMemoPresets,
  saveCustomMemoPresets,
} from '../utils/memoHelpers';

const MEMO_COMPOSE_PRESETS_DEFAULT = [
  { id: 'builtin:0', label: '傲娇女王', text: '以傲娇女王的口气总结一下' },
  { id: 'builtin:1', label: '萝莉语气', text: '以娇滴滴的萝莉语气复述一下' },
  { id: 'builtin:2', label: '项羽霸王', text: '以刚猛项羽霸王的姿态' },
  { id: 'builtin:3', label: '毛式智慧', text: '以毛主席的智慧讲讲' },
];

/**
 * useMemoComposer
 * - 负责：备忘录勾选、引用检测、presets 管理、写入/LLM 优化写入
 *
 * @param {{
 *  open: boolean,
 *  messages: { messageID?: any, role: string, content?: string }[],
 *  onClose: () => void,
 *  onHint?: (text: string) => void,
 *  onError?: (text: string) => void,
 * }} opts
 */
export function useMemoComposer(opts) {
  const { open, messages, onClose, onHint, onError } = opts || {};

  const [memoMarkedIds, setMemoMarkedIds] = useState(() => new Set());
  const memoMarkedIdsRef = useRef(memoMarkedIds);
  memoMarkedIdsRef.current = memoMarkedIds;

  const [memoRefIds, setMemoRefIds] = useState(() => new Set());
  const [memoCheckSaving, setMemoCheckSaving] = useState(false);
  const [memoComposeHint, setMemoComposeHint] = useState('');

  const [customMemoPresets, setCustomMemoPresets] = useState(loadCustomMemoPresets);
  const [memoPresetAddOpen, setMemoPresetAddOpen] = useState(false);
  const [memoPresetDraftLabel, setMemoPresetDraftLabel] = useState('');
  const [memoPresetDraftText, setMemoPresetDraftText] = useState('');

  const allMemoComposePresets = useMemo(
    () => [...MEMO_COMPOSE_PRESETS_DEFAULT, ...customMemoPresets],
    [customMemoPresets],
  );

  const refreshMemoRefIds = useCallback(async () => {
    try {
      const arr = await GetMemoReferencedMessageIDs();
      setMemoRefIds(new Set(Array.isArray(arr) ? arr : []));
    } catch (e) {
      console.error('GetMemoReferencedMessageIDs:', e);
    }
  }, []);

  useEffect(() => {
    if (open) {
      refreshMemoRefIds();
      return;
    }
    setMemoMarkedIds(new Set());
    setMemoComposeHint('');
    setMemoPresetAddOpen(false);
    setMemoPresetDraftLabel('');
    setMemoPresetDraftText('');
  }, [open, refreshMemoRefIds]);

  const tryToggleMemoMark = useCallback(async (messageID) => {
    const id = String(messageID ?? '').trim();
    if (!id) return;

    const prev = memoMarkedIdsRef.current;
    if (prev.has(id)) {
      setMemoMarkedIds((p) => {
        const next = new Set(p);
        next.delete(id);
        return next;
      });
      return;
    }

    if (memoRefIds.has(id)) {
      let ok = false;
      try {
        ok = await ConfirmMemoReinclude();
      } catch (e) {
        console.error('ConfirmMemoReinclude:', e);
        ok = false;
      }
      if (!ok) return;
    }

    setMemoMarkedIds((p) => {
      const next = new Set(p);
      next.add(id);
      return next;
    });
  }, [memoRefIds]);

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

  const finishMemoAppend = useCallback(() => {
    window.dispatchEvent(new CustomEvent('memoSaved', { detail: { focusLatest: true } }));
    onHint?.('已写入备忘录');
    onClose?.();
    setMemoMarkedIds(new Set());
    setMemoComposeHint('');
    refreshMemoRefIds();
  }, [onClose, onHint, refreshMemoRefIds]);

  const saveDirectMemo = useCallback(async () => {
    if (memoCheckSaving) return;
    const list = Array.isArray(messages) ? messages : [];
    const ordered = list.filter((m) => memoMarkedIds.has(String(m.messageID)));
    if (ordered.length === 0) {
      onError?.('请先勾选要写入备忘录的消息。');
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
      onError?.(String(err?.message || err));
    } finally {
      setMemoCheckSaving(false);
    }
  }, [memoCheckSaving, messages, memoMarkedIds, finishMemoAppend, onError]);

  const sendLLMMemo = useCallback(async () => {
    if (memoCheckSaving) return;
    const list = Array.isArray(messages) ? messages : [];
    const ordered = list.filter((m) => memoMarkedIds.has(String(m.messageID)));
    if (ordered.length === 0) {
      onError?.('请先勾选要写入备忘录的消息。');
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
      onError?.(String(err?.message || err));
    } finally {
      setMemoCheckSaving(false);
    }
  }, [memoCheckSaving, messages, memoMarkedIds, memoComposeHint, finishMemoAppend, onError]);

  return {
    memoMarkedIds,
    memoMarkedCount: memoMarkedIds.size,
    memoRefIds,
    memoCheckSaving,
    memoComposeHint,
    setMemoComposeHint,
    allMemoComposePresets,
    memoPresetAddOpen,
    setMemoPresetAddOpen,
    memoPresetDraftLabel,
    setMemoPresetDraftLabel,
    memoPresetDraftText,
    setMemoPresetDraftText,
    addCustomMemoPreset,
    removeCustomMemoPreset,
    tryToggleMemoMark,
    saveDirectMemo,
    sendLLMMemo,
  };
}
