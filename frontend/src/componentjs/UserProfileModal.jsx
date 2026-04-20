import { useCallback, useEffect, useMemo, useState } from 'react';
import { GetUserProfile, RefreshUserProfile } from '../../wailsjs/go/main/App';
import '../componentcss/UserProfileModal.css';

function isSignal(value) {
  return !!value && typeof value === 'object' && ('score' in value || 'confidence' in value || 'evidence' in value);
}

function isRenderableValue(value) {
  if (value === '' || value === null || value === undefined) return false;
  if (Array.isArray(value)) return value.length > 0;
  if (typeof value === 'object') return Object.keys(value).length > 0;
  return true;
}

function chips(items) {
  if (!Array.isArray(items) || items.length === 0) return <span className="uprof-empty-inline">暂无</span>;
  return items.map((item) => (
    <span key={String(item)} className="uprof-chip">
      {String(item)}
    </span>
  ));
}

function signalBlock(value) {
  const score = Number(value?.score ?? 0);
  const confidence = Number(value?.confidence ?? 0);
  const evidence = Array.isArray(value?.evidence) ? value.evidence : [];
  const updatedAt = String(value?.updated_at || '');

  return (
    <div className="uprof-signal">
      <div className="uprof-signal__meters">
        <div className="uprof-meter">
          <div className="uprof-meter__label">Score</div>
          <div className="uprof-meter__track">
            <span className="uprof-meter__fill" style={{ width: `${Math.max(0, Math.min(100, score * 100))}%` }} />
          </div>
          <div className="uprof-meter__value">{score.toFixed(2)}</div>
        </div>
        <div className="uprof-meter">
          <div className="uprof-meter__label">Confidence</div>
          <div className="uprof-meter__track uprof-meter__track--confidence">
            <span className="uprof-meter__fill uprof-meter__fill--confidence" style={{ width: `${Math.max(0, Math.min(100, confidence * 100))}%` }} />
          </div>
          <div className="uprof-meter__value">{confidence.toFixed(2)}</div>
        </div>
      </div>
      {evidence.length ? <div className="uprof-signal__evidence">{chips(evidence)}</div> : null}
      {updatedAt ? <div className="uprof-signal__time">updated {updatedAt}</div> : null}
    </div>
  );
}

function renderValue(value, path = 'root') {
  if (!isRenderableValue(value)) {
    return <span className="uprof-empty-inline">暂无</span>;
  }

  if (isSignal(value)) {
    return signalBlock(value);
  }

  if (Array.isArray(value)) {
    const primitiveItems = value.filter((item) => typeof item !== 'object' || item === null);
    const objectItems = value.filter((item) => typeof item === 'object' && item !== null);

    return (
      <div className="uprof-stack">
        {primitiveItems.length ? <div>{chips(primitiveItems)}</div> : null}
        {objectItems.map((item, idx) => (
          <div key={`${path}-${idx}`} className="uprof-nested-card">
            {renderValue(item, `${path}-${idx}`)}
          </div>
        ))}
      </div>
    );
  }

  if (typeof value === 'object') {
    const entries = Object.entries(value).filter(([, nested]) => isRenderableValue(nested));
    if (!entries.length) return <span className="uprof-empty-inline">暂无</span>;
    return (
      <div className="uprof-stack">
        {entries.map(([nestedKey, nestedValue]) => (
          <div key={`${path}-${nestedKey}`} className="uprof-kv">
            <div className="uprof-kv__key">{nestedKey}</div>
            <div className="uprof-kv__value">{renderValue(nestedValue, `${path}-${nestedKey}`)}</div>
          </div>
        ))}
      </div>
    );
  }

  return <span>{String(value)}</span>;
}

function metricRows(obj) {
  if (!obj || typeof obj !== 'object') return null;
  const entries = Object.entries(obj).filter(([, value]) => isRenderableValue(value));
  if (!entries.length) return <p className="uprof-empty">暂无数据。</p>;
  return (
    <div className="uprof-grid">
      {entries.map(([key, value]) => (
        <div key={key} className="uprof-grid__item">
          <div className="uprof-grid__label">{key}</div>
          <div className="uprof-grid__value">{renderValue(value, key)}</div>
        </div>
      ))}
    </div>
  );
}

export default function UserProfileModal({ open, onClose, chatId = '' }) {
  const [loading, setLoading] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [err, setErr] = useState('');
  const [profile, setProfile] = useState(null);

  const cid = String(chatId || '').trim();

  const load = useCallback(async () => {
    if (!cid) {
      setProfile(null);
      return;
    }
    setLoading(true);
    setErr('');
    try {
      const data = await GetUserProfile(cid);
      setProfile(data && typeof data === 'object' ? data : null);
    } catch (e) {
      setErr(String(e?.message || e));
      setProfile(null);
    } finally {
      setLoading(false);
    }
  }, [cid]);

  const refresh = useCallback(async () => {
    if (!cid) return;
    setRefreshing(true);
    setErr('');
    try {
      const data = await RefreshUserProfile(cid);
      setProfile(data && typeof data === 'object' ? data : null);
    } catch (e) {
      setErr(String(e?.message || e));
    } finally {
      setRefreshing(false);
    }
  }, [cid]);

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

  const memoryItems = useMemo(() => {
    const list = profile?.memory;
    return Array.isArray(list) ? list : [];
  }, [profile]);

  const evidenceWindow = useMemo(() => {
    const list = profile?.source_meta?.evidence_window;
    return Array.isArray(list) ? list : [];
  }, [profile]);

  if (!open) return null;

  return (
    <div className="uprof-overlay" role="presentation" onMouseDown={onClose}>
      <div className="uprof-sheet" role="dialog" aria-labelledby="uprof-title" onMouseDown={(e) => e.stopPropagation()}>
        <div className="uprof-header">
          <div>
            <h2 id="uprof-title" className="uprof-title">
              用户画像
            </h2>
            <p className="uprof-sub">
              这是这台应用长期沉淀的用户画像，当前会话只作为最新补充证据。
            </p>
          </div>
          <div className="uprof-actions">
            <button type="button" className="uprof-btn uprof-btn--ghost" onClick={() => void refresh()} disabled={!cid || refreshing}>
              {refreshing ? '生成中…' : '刷新画像'}
            </button>
            <button type="button" className="uprof-btn uprof-btn--ghost" onClick={onClose}>
              关闭
            </button>
          </div>
        </div>

        {err ? <p className="uprof-error">{err}</p> : null}

        <div className="uprof-body">
          {loading ? (
            <p className="uprof-empty">加载中…</p>
          ) : !cid ? (
            <p className="uprof-empty">请先在左侧选择一个对话。</p>
          ) : !profile?.summary ? (
            <div className="uprof-zero">
              <p className="uprof-empty">当前还没有结构化画像。</p>
              <button type="button" className="uprof-btn" onClick={() => void refresh()} disabled={refreshing}>
                {refreshing ? '生成中…' : '基于历史对话生成画像'}
              </button>
            </div>
          ) : (
            <>
              <section className="uprof-card uprof-card--summary">
                <p className="uprof-summary">{String(profile.summary || '')}</p>
                <div className="uprof-meta">
                  <span>人物画像</span>
                  <span>更新时间: {String(profile.updated_at || '-')}</span>
                  <span>当前会话: {cid || '(未选择对话)'}</span>
                </div>
              </section>

              <section className="uprof-card uprof-card--source">
                <h3>当前会话补充</h3>
                <p className="uprof-source-copy">
                  当前打开的对话会刷新这个人的长期画像，但不会把一次短期情绪直接当成永久特质。
                </p>
                {evidenceWindow.length ? (
                  <div className="uprof-source-chips">{chips(evidenceWindow)}</div>
                ) : (
                  <p className="uprof-empty">暂无本轮补充摘要。</p>
                )}
              </section>

              <section className="uprof-card">
                <h3>长期身份</h3>
                {metricRows(profile.identity)}
              </section>

              <section className="uprof-card">
                <h3>长期偏好</h3>
                {metricRows(profile.preferences)}
              </section>

              <section className="uprof-card">
                <h3>心理画像</h3>
                {metricRows(profile.psychology)}
              </section>

              <section className="uprof-card">
                <h3>下一步推测</h3>
                {metricRows(profile.predictions)}
              </section>

              <section className="uprof-card">
                <h3>长期记忆事件</h3>
                {memoryItems.length === 0 ? (
                  <p className="uprof-empty">暂无关键事件。</p>
                ) : (
                  <ul className="uprof-memory-list">
                    {memoryItems.map((item, idx) => (
                      <li key={`${item?.time || 't'}-${idx}`} className="uprof-memory-item">
                        <div className="uprof-memory-item__meta">
                          <span>{String(item?.time || '-')}</span>
                          <span>{String(item?.type || 'event')}</span>
                          <span>importance {String(item?.importance ?? '-')}</span>
                        </div>
                        <p>{String(item?.summary || '')}</p>
                      </li>
                    ))}
                  </ul>
                )}
              </section>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
