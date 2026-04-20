import { useCallback, useEffect, useMemo, useState } from 'react';
import { DeleteScheduledTask, ListScheduledTasks, SetScheduledTaskStatus, UpdateScheduledTaskBasics } from '../../wailsjs/go/main/App';
import '../componentcss/ScheduledTasksModal.css';

function formatLocalTime(v) {
  if (!v) return '';
  const d = new Date(v);
  if (Number.isNaN(d.getTime())) return '';
  return d.toLocaleString();
}

function weekdayFromBydayToken(tok) {
  switch (String(tok || '').trim().toUpperCase()) {
    case 'MO':
      return '周一';
    case 'TU':
      return '周二';
    case 'WE':
      return '周三';
    case 'TH':
      return '周四';
    case 'FR':
      return '周五';
    case 'SA':
      return '周六';
    case 'SU':
      return '周日';
    default:
      return '';
  }
}

function describeSchedule(it) {
  const scheduleType = String(it?.schedule_type ?? '').trim().toLowerCase();
  const tz = String(it?.timezone ?? '').trim();
  const runAt = formatLocalTime(it?.run_at);
  const cron = String(it?.cron_expr ?? '').trim();
  const rrule = String(it?.rrule ?? '').trim();

  if (scheduleType === 'once') {
    return runAt ? `一次性：${runAt}${tz ? `（${tz}）` : ''}` : `一次性${tz ? `（${tz}）` : ''}`;
  }

  // RRULE minimal humanization.
  if (rrule) {
    const parts = Object.fromEntries(
      rrule
        .split(';')
        .map((kv) => kv.split('='))
        .filter((kv) => kv.length === 2)
        .map(([k, v]) => [String(k).toUpperCase(), String(v)]),
    );
    const freq = String(parts.FREQ || '').toUpperCase();
    const interval = Number(parts.INTERVAL || 1);
    const byday = String(parts.BYDAY || '');
    const days = byday
      ? byday
          .split(',')
          .map(weekdayFromBydayToken)
          .filter(Boolean)
          .join('、')
      : '';
    if (freq === 'DAILY') {
      return `每天${interval > 1 ? `（每 ${interval} 天）` : ''}${tz ? `（${tz}）` : ''}`;
    }
    if (freq === 'HOURLY') {
      return `每 ${interval || 1} 小时${tz ? `（${tz}）` : ''}`;
    }
    if (freq === 'WEEKLY') {
      return `每周${days ? `（${days}）` : ''}${interval > 1 ? `，每 ${interval} 周` : ''}${tz ? `（${tz}）` : ''}`;
    }
    return `RRULE：${rrule}${tz ? `（${tz}）` : ''}`;
  }

  // CRON minimal humanization: "0 8 * * *" => 每天 08:00
  if (cron) {
    const seg = cron.split(/\s+/).filter(Boolean);
    if (seg.length === 5 && seg[0] === '0' && /^\d+$/.test(seg[1]) && seg[2] === '*' && seg[3] === '*' && seg[4] === '*') {
      const h = String(seg[1]).padStart(2, '0');
      return `每天 ${h}:00${tz ? `（${tz}）` : ''}`;
    }
    return `CRON：${cron}${tz ? `（${tz}）` : ''}`;
  }

  return tz ? `周期任务（${tz}）` : '周期任务';
}

function stripTzSuffix(text) {
  const s = String(text || '').trim();
  if (!s) return '';
  return s.replace(/（[^）]+）\s*$/, '').trim();
}

function badgeLabelForStatus(status) {
  const s = String(status || '').trim().toLowerCase();
  if (s === 'active') return '已激活';
  if (s === 'paused') return '已暂停';
  if (s === 'deleted') return '已删除';
  return s || '未知';
}

function scheduleLabel(scheduleType) {
  const s = String(scheduleType || '').trim().toLowerCase();
  if (s === 'once') return '一次性';
  if (s === 'recurring') return '周期';
  return s || '未知';
}

function actionLabel(actionType) {
  const s = String(actionType || '').trim().toLowerCase();
  if (s === 'notify') return '提醒';
  if (s === 'tool') return '调用工具';
  return s || '动作';
}

export default function ScheduledTasksModal({ open, onClose }) {
  const [loading, setLoading] = useState(false);
  const [err, setErr] = useState('');
  const [items, setItems] = useState([]);
  const [includeDeleted, setIncludeDeleted] = useState(false);
  const [busyId, setBusyId] = useState('');
  const [editId, setEditId] = useState('');
  const [confirmDeleteId, setConfirmDeleteId] = useState('');
  const [draftTitle, setDraftTitle] = useState('');
  const [draftPayload, setDraftPayload] = useState('');

  const load = useCallback(async () => {
    setErr('');
    setLoading(true);
    try {
      const list = await ListScheduledTasks('', includeDeleted, 200, 0);
      setItems(Array.isArray(list) ? list : []);
    } catch (e) {
      setItems([]);
      setErr(String(e?.message || e));
    } finally {
      setLoading(false);
    }
  }, [includeDeleted]);

  useEffect(() => {
    if (!open) return;
    void load();
  }, [open, load]);

  useEffect(() => {
    if (!open) return;
    const onKey = (e) => {
      if (e.key === 'Escape') onClose?.();
    };
    window.addEventListener('keydown', onKey);
    return () => window.removeEventListener('keydown', onKey);
  }, [open, onClose]);

  const summary = useMemo(() => `${items.length} 条`, [items]);

  const startEdit = useCallback((it) => {
    const id = String(it?.id ?? '');
    if (!id) return;
    setEditId(id);
    setDraftTitle(String(it?.title ?? ''));
    setDraftPayload(String(it?.action_payload ?? ''));
  }, []);

  const cancelEdit = useCallback(() => {
    setEditId('');
    setDraftTitle('');
    setDraftPayload('');
  }, []);

  const cancelDelete = useCallback(() => {
    setConfirmDeleteId('');
  }, []);

  const doDelete = useCallback(
    async (id) => {
      const tid = String(id || '').trim();
      if (!tid) return;
      setBusyId(tid);
      setErr('');
      try {
        await DeleteScheduledTask(tid);
        setConfirmDeleteId('');
        await load();
      } catch (e) {
        setErr(String(e?.message || e));
      } finally {
        setBusyId('');
      }
    },
    [load],
  );

  const togglePause = useCallback(
    async (it) => {
      const tid = String(it?.id ?? '').trim();
      if (!tid) return;
      const cur = String(it?.status ?? '').trim().toLowerCase();
      const next = cur === 'paused' ? 'active' : 'paused';
      setBusyId(tid);
      setErr('');
      try {
        await SetScheduledTaskStatus(tid, next);
        await load();
      } catch (e) {
        setErr(String(e?.message || e));
      } finally {
        setBusyId('');
      }
    },
    [load],
  );

  const saveBasics = useCallback(
    async (id) => {
      const tid = String(id || '').trim();
      if (!tid) return;
      const t = String(draftTitle || '').trim();
      const p = String(draftPayload || '').trim();
      if (!t) {
        setErr('标题不能为空');
        return;
      }
      setBusyId(tid);
      setErr('');
      try {
        await UpdateScheduledTaskBasics(tid, t, p);
        cancelEdit();
        await load();
      } catch (e) {
        setErr(String(e?.message || e));
      } finally {
        setBusyId('');
      }
    },
    [draftTitle, draftPayload, load, cancelEdit],
  );

  if (!open) return null;

  return (
    <div className="stask-overlay" role="presentation" onMouseDown={onClose}>
      <div className="stask-sheet" role="dialog" aria-labelledby="stask-title" onMouseDown={(e) => e.stopPropagation()}>
        <div className="stask-sheet__header">
          <div className="stask-sheet__title-wrap">
            <h2 id="stask-title" className="stask-sheet__title">
              定时任务
            </h2>
            <p className="stask-sheet__sub">共 {summary}</p>
          </div>
          <div className="stask-sheet__actions">
            <label className="stask-toggle" title="包含已删除任务">
              <input type="checkbox" checked={includeDeleted} onChange={(e) => setIncludeDeleted(e.target.checked)} />
              <span>含已删除</span>
            </label>
            <button type="button" className="stask-btn stask-btn--ghost" onClick={() => void load()} disabled={loading} title="刷新">
              ↻
            </button>
            <button type="button" className="stask-btn stask-btn--ghost" onClick={onClose}>
              关闭
            </button>
          </div>
        </div>

        {err ? <p className="stask-sheet__error">{err}</p> : null}

        <div className="stask-body">
          {loading ? (
            <p className="stask-empty">加载中…</p>
          ) : items.length === 0 ? (
            <p className="stask-empty">暂无定时任务。</p>
          ) : (
            <ul className="stask-list" aria-label="定时任务列表">
              {items.map((it) => {
                const id = String(it?.id ?? '');
                const title = String(it?.title ?? '');
                const status = String(it?.status ?? '');
                const timezone = String(it?.timezone ?? '');
                const next = formatLocalTime(it?.next_run_at);
                const last = formatLocalTime(it?.last_run_at);
                const actionType = String(it?.action_type ?? '');
                const actionPayload = String(it?.action_payload ?? '');
                const executing = Boolean(it?.executing);
                const completed = Boolean(it?.completed);
                const runCount = Number(it?.run_count ?? 0);

                const statusLabel = badgeLabelForStatus(status);
                const actionTypeLabel = actionLabel(actionType);
                const scheduleText = describeSchedule(it);
                const scheduleTextNoTz = stripTzSuffix(scheduleText);

                const notifyText = actionType === 'notify' ? String(actionPayload || '').trim() : '';
                const isBusy = busyId && busyId === id;
                const isEditing = editId && editId === id;
                const isConfirmingDelete = confirmDeleteId && confirmDeleteId === id;

                return (
                  <li key={id} className="stask-item">
                    <div className="stask-item__top">
                      <div className="stask-item__title" title={title || id}>
                        {title || '(无标题)'}
                      </div>
                      <div className="stask-badges">
                        {executing ? <span className="stask-badge stask-badge--running">执行中</span> : null}
                        {completed ? <span className="stask-badge stask-badge--muted">已完成</span> : null}
                        <span className={`stask-badge ${String(status || '').toLowerCase() === 'active' ? 'stask-badge--active' : ''}`}>{statusLabel}</span>
                      </div>
                    </div>
                    <ul className="stask-bullets" aria-label="定时任务摘要">
                      <li>
                        <span className="stask-bullets__k">⏰ 触发时间：</span>
                        <span className="stask-bullets__v">{scheduleTextNoTz || scheduleText}</span>
                      </li>
                      <li>
                        <span className="stask-bullets__k">🔔 {actionTypeLabel}内容：</span>
                        <span className="stask-bullets__v">{notifyText || '-'}</span>
                      </li>
                      <li>
                        <span className="stask-bullets__k">🗓 下一次触发：</span>
                        <span className="stask-bullets__v stask-bullets__v--primary">{next || '-'}</span>
                      </li>
                      <li>
                        <span className="stask-bullets__k">📈 执行次数：</span>
                        <span className="stask-bullets__v">{Number.isFinite(runCount) ? runCount : 0}</span>
                      </li>
                      <li>
                        <span className="stask-bullets__k">✅ 状态：</span>
                        <span className="stask-bullets__v">{statusLabel}</span>
                      </li>
                      <li>
                        <span className="stask-bullets__k">🕘 上一次触发：</span>
                        <span className="stask-bullets__v">{last || '-'}</span>
                      </li>
                    </ul>
                    <div className="stask-item__actionsRow" aria-label="任务操作">
                      <button
                        type="button"
                        className="stask-iconbtn"
                        title={String(status || '').toLowerCase() === 'paused' ? '恢复' : '暂停'}
                        onClick={() => void togglePause(it)}
                        disabled={isBusy || executing}
                      >
                        {String(status || '').toLowerCase() === 'paused' ? '▶︎' : '⏸'}
                      </button>
                      <button type="button" className="stask-iconbtn" title="修改" onClick={() => startEdit(it)} disabled={isBusy || executing}>
                        ✎
                      </button>
                      {isConfirmingDelete ? (
                        <div className="stask-confirm">
                          <button type="button" className="stask-confirm__btn" onClick={cancelDelete} disabled={isBusy}>
                            取消
                          </button>
                          <button type="button" className="stask-confirm__btn stask-confirm__btn--danger" onClick={() => void doDelete(id)} disabled={isBusy || executing}>
                            确认删除
                          </button>
                        </div>
                      ) : (
                        <button
                          type="button"
                          className="stask-iconbtn stask-iconbtn--danger"
                          title="删除"
                          onClick={() => setConfirmDeleteId(id)}
                          disabled={isBusy || executing}
                        >
                          🗑
                        </button>
                      )}
                    </div>

                    {isEditing ? (
                      <div className="stask-edit" aria-label="编辑定时任务">
                        <div className="stask-edit__row">
                          <label className="stask-edit__label">
                            标题
                            <input className="stask-input" value={draftTitle} onChange={(e) => setDraftTitle(e.target.value)} />
                          </label>
                          <label className="stask-edit__label">
                            提醒内容
                            <input className="stask-input" value={draftPayload} onChange={(e) => setDraftPayload(e.target.value)} />
                          </label>
                        </div>
                        <div className="stask-edit__actions">
                          <button type="button" className="stask-btn stask-btn--ghost" onClick={cancelEdit} disabled={isBusy}>
                            取消
                          </button>
                          <button type="button" className="stask-btn stask-btn--primary" onClick={() => void saveBasics(id)} disabled={isBusy}>
                            保存
                          </button>
                        </div>
                      </div>
                    ) : null}
                  </li>
                );
              })}
            </ul>
          )}
        </div>
      </div>
    </div>
  );
}

