import { useCallback, useEffect, useMemo, useState } from 'react';
import {
  DeleteScheduledTask,
  ListScheduledTasks,
  SetScheduledTaskStatus,
  UpdateScheduledTaskBasics,
} from '../../../wailsjs/go/main/App';
import '../../styles/scheduled-tasks.css';

const dateTimeFormatter = new Intl.DateTimeFormat('zh-CN', {
  year: 'numeric',
  month: 'short',
  day: 'numeric',
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
});

const timeFormatter = new Intl.DateTimeFormat('zh-CN', {
  hour: '2-digit',
  minute: '2-digit',
  hour12: false,
});

const taskStateMeta = {
  active: { label: '运行中', rank: 1 },
  executing: { label: '执行中', rank: 0 },
  paused: { label: '已暂停', rank: 2 },
  completed: { label: '已完成', rank: 3 },
  deleted: { label: '已删除', rank: 4 },
  unknown: { label: '状态未知', rank: 5 },
};

function toDate(value) {
  if (!value) return null;
  const date = new Date(value);
  return Number.isNaN(date.getTime()) ? null : date;
}

function formatLocalTime(value) {
  const date = toDate(value);
  return date ? dateTimeFormatter.format(date).replace(/\s+/g, ' ') : '';
}

function formatNextTime(value) {
  const date = toDate(value);
  if (!date) return '无后续执行';

  const now = new Date();
  const startOfToday = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const startOfTarget = new Date(date.getFullYear(), date.getMonth(), date.getDate());
  const dayDiff = Math.round((startOfTarget.getTime() - startOfToday.getTime()) / 86400000);
  const clock = timeFormatter.format(date);

  if (dayDiff === 0) return `今天 ${clock}`;
  if (dayDiff === 1) return `明天 ${clock}`;
  if (date.getFullYear() === now.getFullYear()) return `${date.getMonth() + 1}月${date.getDate()}日 ${clock}`;
  return `${date.getFullYear()}年${date.getMonth() + 1}月${date.getDate()}日 ${clock}`;
}

function weekdayFromBydayToken(token) {
  return {
    MO: '周一',
    TU: '周二',
    WE: '周三',
    TH: '周四',
    FR: '周五',
    SA: '周六',
    SU: '周日',
  }[String(token || '').trim().toUpperCase()] || '';
}

function describeSchedule(item) {
  const scheduleType = String(item?.schedule_type ?? '').trim().toLowerCase();
  const runAt = formatLocalTime(item?.run_at);
  const cron = String(item?.cron_expr ?? '').trim();
  const rrule = String(item?.rrule ?? '').trim();

  if (scheduleType === 'once') return runAt ? `一次性 · ${runAt}` : '一次性任务';

  if (rrule) {
    const parts = Object.fromEntries(
      rrule
        .split(';')
        .map((entry) => entry.split('='))
        .filter((entry) => entry.length === 2)
        .map(([key, value]) => [String(key).toUpperCase(), String(value)]),
    );
    const frequency = String(parts.FREQ || '').toUpperCase();
    const interval = Math.max(Number(parts.INTERVAL || 1), 1);
    const days = String(parts.BYDAY || '')
      .split(',')
      .map(weekdayFromBydayToken)
      .filter(Boolean)
      .join('、');

    if (frequency === 'HOURLY') return interval === 1 ? '每小时' : `每 ${interval} 小时`;
    if (frequency === 'DAILY') return interval === 1 ? '每天' : `每 ${interval} 天`;
    if (frequency === 'WEEKLY') {
      const prefix = interval === 1 ? '每周' : `每 ${interval} 周`;
      return days ? `${prefix} · ${days}` : prefix;
    }
    return `RRULE · ${rrule}`;
  }

  if (cron) {
    const segments = cron.split(/\s+/).filter(Boolean);
    if (segments.length === 5) {
      const [minute, hour, day, month, weekday] = segments;
      if (/^\d+$/.test(minute) && /^\d+$/.test(hour)) {
        const clock = `${String(hour).padStart(2, '0')}:${String(minute).padStart(2, '0')}`;
        if (day === '*' && month === '*' && weekday === '*') return `每天 ${clock}`;
        if (/^\d+$/.test(day) && /^\d+$/.test(month) && weekday === '*') return `每年 ${month}月${day}日 ${clock}`;
      }
    }
    return `CRON · ${cron}`;
  }

  return '周期任务';
}

function taskState(item) {
  const status = String(item?.status ?? '').trim().toLowerCase();
  if (status === 'deleted') return 'deleted';
  if (Boolean(item?.executing)) return 'executing';
  if (Boolean(item?.completed)) return 'completed';
  if (status === 'active' || status === 'paused') return status;
  return 'unknown';
}

function actionLabel(actionType) {
  return String(actionType || '').trim().toLowerCase() === 'tool' ? '工具任务' : '提醒';
}

function actionSummary(item) {
  const payload = String(item?.action_payload ?? '').trim();
  if (!payload) return '未填写任务内容';
  if (String(item?.action_type ?? '').trim().toLowerCase() !== 'tool') return payload;

  try {
    const parsed = JSON.parse(payload);
    const toolName = String(parsed?.tool_name ?? parsed?.tool ?? parsed?.name ?? '').trim();
    return toolName ? `调用 ${toolName}` : payload;
  } catch {
    return payload;
  }
}

function taskTimeValue(item, key) {
  return toDate(item?.[key])?.getTime() || 0;
}

function compareTasks(left, right) {
  const leftState = taskState(left);
  const rightState = taskState(right);
  const rankDiff = taskStateMeta[leftState].rank - taskStateMeta[rightState].rank;
  if (rankDiff !== 0) return rankDiff;

  if (leftState === 'active' || leftState === 'executing') {
    const leftNext = taskTimeValue(left, 'next_run_at') || Number.MAX_SAFE_INTEGER;
    const rightNext = taskTimeValue(right, 'next_run_at') || Number.MAX_SAFE_INTEGER;
    if (leftNext !== rightNext) return leftNext - rightNext;
  }
  return taskTimeValue(right, 'updated_at') - taskTimeValue(left, 'updated_at');
}

function TaskActionIcon({ name }) {
  const commonProps = {
    'aria-hidden': true,
    className: 'stask-action__icon',
    fill: 'none',
    viewBox: '0 0 20 20',
  };

  if (name === 'pause') {
    return (
      <svg {...commonProps}>
        <path d="M7 5v10M13 5v10" />
      </svg>
    );
  }
  if (name === 'resume') {
    return (
      <svg {...commonProps}>
        <path d="m7 5 7 5-7 5V5Z" />
      </svg>
    );
  }
  if (name === 'edit') {
    return (
      <svg {...commonProps}>
        <path d="m12.8 4.2 3 3M5 15l2.7-.5 7.6-7.6a1.3 1.3 0 0 0 0-1.8l-.4-.4a1.3 1.3 0 0 0-1.8 0l-7.6 7.6L5 15Z" />
      </svg>
    );
  }
  return (
    <svg {...commonProps}>
      <path d="M4.5 6h11M8 3.5h4M6.5 6l.6 10h5.8l.6-10M8.5 8.5v5M11.5 8.5v5" />
    </svg>
  );
}

export default function ScheduledTasksModal({ open, onClose }) {
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState('');
  const [items, setItems] = useState([]);
  const [includeDeleted, setIncludeDeleted] = useState(false);
  const [query, setQuery] = useState('');
  const [statusFilter, setStatusFilter] = useState('all');
  const [busyId, setBusyId] = useState('');
  const [editId, setEditId] = useState('');
  const [confirmDeleteId, setConfirmDeleteId] = useState('');
  const [draftTitle, setDraftTitle] = useState('');
  const [draftPayload, setDraftPayload] = useState('');

  const load = useCallback(async () => {
    setError('');
    setLoading(true);
    try {
      const list = await ListScheduledTasks('', includeDeleted, 200, 0);
      setItems(Array.isArray(list) ? list : []);
    } catch (loadError) {
      setError(String(loadError?.message || loadError));
    } finally {
      setLoading(false);
    }
  }, [includeDeleted]);

  const startEdit = useCallback((item) => {
    const id = String(item?.id ?? '').trim();
    if (!id) return;
    setEditId(id);
    setConfirmDeleteId('');
    setDraftTitle(String(item?.title ?? ''));
    setDraftPayload(String(item?.action_payload ?? ''));
  }, []);

  const cancelEdit = useCallback(() => {
    setEditId('');
    setDraftTitle('');
    setDraftPayload('');
  }, []);

  const doDelete = useCallback(
    async (id) => {
      const taskId = String(id || '').trim();
      if (!taskId) return;
      setBusyId(taskId);
      setError('');
      try {
        await DeleteScheduledTask(taskId);
        setConfirmDeleteId('');
        await load();
      } catch (deleteError) {
        setError(String(deleteError?.message || deleteError));
      } finally {
        setBusyId('');
      }
    },
    [load],
  );

  const togglePause = useCallback(
    async (item) => {
      const taskId = String(item?.id ?? '').trim();
      if (!taskId) return;
      const nextStatus = String(item?.status ?? '').trim().toLowerCase() === 'paused' ? 'active' : 'paused';
      setBusyId(taskId);
      setError('');
      try {
        await SetScheduledTaskStatus(taskId, nextStatus);
        await load();
      } catch (statusError) {
        setError(String(statusError?.message || statusError));
      } finally {
        setBusyId('');
      }
    },
    [load],
  );

  const saveBasics = useCallback(
    async (id) => {
      const taskId = String(id || '').trim();
      const title = String(draftTitle || '').trim();
      const payload = String(draftPayload || '').trim();
      if (!taskId) return;
      if (!title) {
        setError('标题不能为空');
        return;
      }

      setBusyId(taskId);
      setError('');
      try {
        await UpdateScheduledTaskBasics(taskId, title, payload);
        cancelEdit();
        await load();
      } catch (saveError) {
        setError(String(saveError?.message || saveError));
      } finally {
        setBusyId('');
      }
    },
    [cancelEdit, draftPayload, draftTitle, load],
  );

  useEffect(() => {
    if (open) void load();
  }, [open, load]);

  useEffect(() => {
    if (!open) return undefined;
    const onKeyDown = (event) => {
      if (event.key !== 'Escape') return;
      if (confirmDeleteId) {
        setConfirmDeleteId('');
      } else if (editId) {
        cancelEdit();
      } else {
        onClose?.();
      }
    };
    window.addEventListener('keydown', onKeyDown);
    return () => window.removeEventListener('keydown', onKeyDown);
  }, [cancelEdit, confirmDeleteId, editId, onClose, open]);

  const taskView = useMemo(() => {
    const counts = { active: 0, executing: 0, paused: 0, completed: 0, deleted: 0, unknown: 0 };
    const needle = query.trim().toLowerCase();
    const visible = [];

    for (const item of items) {
      const state = taskState(item);
      counts[state] += 1;
      if (statusFilter !== 'all' && state !== statusFilter) continue;

      if (needle) {
        const searchable = [item?.title, item?.action_payload, describeSchedule(item), item?.timezone]
          .map((value) => String(value ?? '').toLowerCase())
          .join('\n');
        if (!searchable.includes(needle)) continue;
      }
      visible.push(item);
    }

    visible.sort(compareTasks);
    return { counts, visible };
  }, [items, query, statusFilter]);

  if (!open) return null;

  const initialLoading = loading && items.length === 0;

  return (
    <div className="stask-overlay" role="presentation" onMouseDown={onClose}>
      <section
        className="stask-sheet"
        role="dialog"
        aria-modal="true"
        aria-labelledby="stask-title"
        aria-busy={loading}
        onMouseDown={(event) => event.stopPropagation()}
      >
        <header className="stask-sheet__header">
          <div className="stask-sheet__heading">
            <h2 id="stask-title" className="stask-sheet__title">定时任务</h2>
            <span className="stask-sheet__count" aria-live="polite">
              {items.length} 个任务，{taskView.counts.active + taskView.counts.executing} 个运行中
            </span>
          </div>
          <button type="button" className="stask-close" onClick={onClose} aria-label="关闭定时任务">×</button>
        </header>

        <div className="stask-toolbar">
          <label className="stask-search">
            <span className="stask-visually-hidden">搜索任务</span>
            <input
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="搜索标题或内容"
              autoFocus
            />
          </label>

          <label className="stask-filter">
            <span>状态</span>
            <select value={statusFilter} onChange={(event) => setStatusFilter(event.target.value)}>
              <option value="all">全部</option>
              <option value="active">运行中</option>
              <option value="executing">执行中</option>
              <option value="paused">已暂停</option>
              <option value="completed">已完成</option>
              {includeDeleted ? <option value="deleted">已删除</option> : null}
            </select>
          </label>

          <label className="stask-check">
            <input
              type="checkbox"
              checked={includeDeleted}
              onChange={(event) => {
                const checked = event.target.checked;
                setIncludeDeleted(checked);
                if (!checked && statusFilter === 'deleted') setStatusFilter('all');
              }}
            />
            <span>显示已删除</span>
          </label>

          <button type="button" className="stask-button" onClick={() => void load()} disabled={loading}>
            {loading ? '刷新中…' : '刷新'}
          </button>
        </div>

        {error ? <p className="stask-sheet__error" role="alert">{error}</p> : null}

        <div className="stask-body">
          {initialLoading ? (
            <p className="stask-empty">正在加载任务…</p>
          ) : taskView.visible.length === 0 ? (
            <div className="stask-empty">
              <strong>{items.length === 0 ? '还没有定时任务' : '没有匹配的任务'}</strong>
              <span>{items.length === 0 ? '可以在对话中让工具人创建提醒或周期任务。' : '尝试清空搜索词或切换状态。'}</span>
            </div>
          ) : (
            <ul className="stask-list" aria-label="定时任务列表">
              {taskView.visible.map((item) => {
                const id = String(item?.id ?? '').trim();
                const title = String(item?.title ?? '').trim() || '无标题任务';
                const state = taskState(item);
                const stateMeta = taskStateMeta[state];
                const schedule = describeSchedule(item);
                const summary = actionSummary(item);
                const nextRun = formatNextTime(item?.next_run_at);
                const runCount = Number.isFinite(Number(item?.run_count)) ? Number(item.run_count) : 0;
                const isBusy = busyId === id;
                const isEditing = editId === id;
                const isConfirmingDelete = confirmDeleteId === id;
                const canToggle = (state === 'active' || state === 'paused') && !Boolean(item?.completed);
                const canChange = state !== 'deleted' && !Boolean(item?.executing);

                return (
                  <li key={id} className={`stask-row stask-row--${state}${isBusy ? ' stask-row--busy' : ''}`}>
                    <div className="stask-row__main">
                      <div className="stask-row__content">
                        <div className="stask-row__title-line">
                          <span className={`stask-status stask-status--${state}`}>{stateMeta.label}</span>
                          <h3 title={title}>{title}</h3>
                        </div>
                        <p className="stask-row__summary" title={summary}>{summary}</p>
                        <div className="stask-row__meta">
                          <span>{actionLabel(item?.action_type)}</span>
                          <span>{schedule}</span>
                          <span>已执行 {runCount} 次</span>
                        </div>
                      </div>

                      <div className="stask-row__next">
                        <span>下次执行</span>
                        <strong>{nextRun}</strong>
                      </div>

                      <div className="stask-row__actions" aria-label={`${title}的操作`}>
                        {canToggle ? (
                          <button
                            type="button"
                            className="stask-action"
                            aria-label={state === 'paused' ? '恢复任务' : '暂停任务'}
                            title={state === 'paused' ? '恢复' : '暂停'}
                            onClick={() => void togglePause(item)}
                            disabled={isBusy}
                          >
                            <TaskActionIcon name={state === 'paused' ? 'resume' : 'pause'} />
                          </button>
                        ) : null}
                        {canChange ? (
                          <button
                            type="button"
                            className="stask-action"
                            aria-label="编辑任务"
                            title="编辑"
                            onClick={() => startEdit(item)}
                            disabled={isBusy}
                          >
                            <TaskActionIcon name="edit" />
                          </button>
                        ) : null}
                        {canChange ? (
                          <button
                            type="button"
                            className="stask-action stask-action--danger"
                            aria-label="删除任务"
                            title="删除"
                            onClick={() => {
                              setConfirmDeleteId(id);
                              setEditId('');
                            }}
                            disabled={isBusy}
                          >
                            <TaskActionIcon name="delete" />
                          </button>
                        ) : null}
                      </div>
                    </div>

                    {isEditing ? (
                      <div className="stask-edit" aria-label="编辑定时任务">
                        <label>
                          <span>标题</span>
                          <input value={draftTitle} onChange={(event) => setDraftTitle(event.target.value)} />
                        </label>
                        <label>
                          <span>{actionLabel(item?.action_type)}内容</span>
                          <textarea rows={3} value={draftPayload} onChange={(event) => setDraftPayload(event.target.value)} />
                        </label>
                        <div className="stask-edit__actions">
                          <button type="button" className="stask-button" onClick={cancelEdit} disabled={isBusy}>取消</button>
                          <button type="button" className="stask-button stask-button--primary" onClick={() => void saveBasics(id)} disabled={isBusy}>
                            {isBusy ? '保存中…' : '保存修改'}
                          </button>
                        </div>
                      </div>
                    ) : null}

                    {isConfirmingDelete ? (
                      <div className="stask-delete-confirm" role="alert">
                        <span>确定删除“{title}”？删除后任务不会再执行。</span>
                        <div>
                          <button type="button" className="stask-button" onClick={() => setConfirmDeleteId('')} disabled={isBusy}>取消</button>
                          <button type="button" className="stask-button stask-button--danger" onClick={() => void doDelete(id)} disabled={isBusy}>
                            {isBusy ? '删除中…' : '确认删除'}
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
      </section>
    </div>
  );
}
