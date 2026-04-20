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
              chatID: <code>{cid || '(未选择对话)'}</code>
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
                  <span>更新时间: {String(profile.updated_at || '-')}</span>
                  <span>Schema: {String(profile.schema_version || '-')}</span>
                </div>
              </section>

              <section className="uprof-card">
                <h3>Identity</h3>
                {metricRows(profile.identity)}
              </section>

              <section className="uprof-card">
                <h3>Preferences</h3>
                {metricRows(profile.preferences)}
              </section>

              <section className="uprof-card">
                <h3>Personality</h3>
                {metricRows(profile.personality)}
              </section>

              <section className="uprof-card">
                <h3>Behavior</h3>
                {metricRows(profile.behavior)}
              </section>

              <section className="uprof-card">
                <h3>State</h3>
                {metricRows(profile.state)}
              </section>

              <section className="uprof-card">
                <h3>Predictions</h3>
                {metricRows(profile.predictions)}
              </section>

              <section className="uprof-card">
                <h3>Memory Events</h3>
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
