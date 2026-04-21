import { useCallback, useEffect, useMemo, useState } from 'react';
import { GetUserProfile, RefreshUserProfile } from '../../wailsjs/go/main/App';
import '../componentcss/UserProfileModal.css';

/** 画像 JSON 字段名 → 中文展示（仅影响文案，不改数据） */
const PROFILE_FIELD_ZH = {
  schema_version: '模式版本',
  person_id: '人物标识',
  chat_id: '会话标识',
  updated_at: '更新时间',
  summary: '摘要',
  identity: '身份',
  preferences: '偏好',
  psychology: '心理',
  memory: '记忆',
  predictions: '推测',
  source_meta: '来源元数据',
  age_range: '年龄段',
  gender: '性别',
  location: '所在地',
  occupation: '职业',
  industry: '行业',
  education: '教育程度',
  language: '语言',
  technical_level: '技术水平',
  active_time_range: '活跃时段',
  interests: '兴趣',
  disliked_topics: '不喜欢的话题',
  content_preference: '内容偏好',
  information_density_preference: '信息密度偏好',
  reasoning_depth_preference: '推理深度偏好',
  preferred_response_pattern: '偏好回应方式',
  tool_usage_tendency: '工具使用倾向',
  response_signals: '回应相关信号',
  traits: '特质',
  state: '状态',
  motivations: '动机',
  behavior_style: '行为风格',
  observations: '观察记录',
  recurrent_themes: '反复出现的主题',
  unresolved_conflicts: '未化解的冲突',
  emotional_triggers: '情绪触发点',
  soothing_patterns: '安抚方式',
  resistance_patterns: '抵触模式',
  identity_narrative: '身份叙事',
  likely_next_topics: '可能下一轮话题',
  likely_next_action: '可能下一步行动',
  signals: '信号',
  time: '时间',
  type: '类型',
  importance: '重要度',
  evidence: '依据',
  score: '得分',
  confidence: '置信度',
  user_message_count_analyzed: '已分析用户消息数',
  assistant_message_count: '助手消息数',
  last_message_at: '最后消息时间',
  generated_from: '生成来源',
  evidence_window: '证据窗口',
  openness: '开放性',
  conscientiousness: '尽责性',
  extraversion: '外向性',
  agreeableness: '宜人性',
  neuroticism: '神经质倾向',
  stress_level: '压力水平',
  need_for_structure: '对结构的需求',
  meaning_seeking: '意义寻求',
  analysis_before_action: '先分析再行动',
  churn_risk: '流失风险',
};

const PROFILE_KEY_PART_ZH = {
  need: '需求',
  for: '',
  structure: '结构',
  stress: '压力',
  level: '水平',
  risk: '风险',
  churn: '流失',
  user: '用户',
  message: '消息',
  count: '计数',
  analyzed: '已分析',
  assistant: '助手',
  last: '最后',
  at: '时间',
  generated: '生成',
  from: '来源',
  window: '窗口',
  depth: '深度',
  reasoning: '推理',
  information: '信息',
  density: '密度',
  preference: '偏好',
  pattern: '模式',
  usage: '使用',
  tool: '工具',
  response: '回应',
  time: '时间',
  range: '范围',
  active: '活跃',
  technical: '技术',
  emotional: '情绪',
  triggers: '触发',
  recurrent: '反复',
  themes: '主题',
  unresolved: '未化解',
  conflicts: '冲突',
  soothing: '安抚',
  resistance: '抵触',
  narrative: '叙事',
  identity: '身份',
  topics: '话题',
  likely: '可能',
  next: '下一',
  action: '行动',
  seeking: '寻求',
  meaning: '意义',
  analysis: '分析',
  before: '先于',
  behavior: '行为',
  style: '风格',
  motivation: '动机',
  observations: '观察',
  content: '内容',
  disliked: '不喜欢',
  interests: '兴趣',
  occupation: '职业',
  education: '教育',
  industry: '行业',
  location: '地区',
  language: '语言',
};

function zhProfileFieldKey(key) {
  if (key == null || key === '') return key;
  const s = String(key);
  if (PROFILE_FIELD_ZH[s]) return PROFILE_FIELD_ZH[s];
  if (s.includes('_')) {
    const parts = s.split('_').filter(Boolean);
    const mapped = parts.map((p) => PROFILE_KEY_PART_ZH[p] ?? p).filter(Boolean);
    if (mapped.length) return mapped.join('·');
  }
  return s;
}

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
          <div className="uprof-meter__label">得分</div>
          <div className="uprof-meter__track">
            <span className="uprof-meter__fill" style={{ width: `${Math.max(0, Math.min(100, score * 100))}%` }} />
          </div>
          <div className="uprof-meter__value">{score.toFixed(2)}</div>
        </div>
        <div className="uprof-meter">
          <div className="uprof-meter__label">置信度</div>
          <div className="uprof-meter__track uprof-meter__track--confidence">
            <span className="uprof-meter__fill uprof-meter__fill--confidence" style={{ width: `${Math.max(0, Math.min(100, confidence * 100))}%` }} />
          </div>
          <div className="uprof-meter__value">{confidence.toFixed(2)}</div>
        </div>
      </div>
      {evidence.length ? <div className="uprof-signal__evidence">{chips(evidence)}</div> : null}
      {updatedAt ? <div className="uprof-signal__time">更新于 {updatedAt}</div> : null}
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
            <div className="uprof-kv__key">{zhProfileFieldKey(nestedKey)}</div>
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
          <div className="uprof-grid__label">{zhProfileFieldKey(key)}</div>
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

              <div className="uprof-board">
                <div className="uprof-board__main">
                  <section className="uprof-card">
                    <h3>长期身份</h3>
                    {metricRows(profile.identity)}
                  </section>

                  <section className="uprof-card">
                    <h3>长期偏好</h3>
                    {metricRows(profile.preferences)}
                  </section>

                  <section className="uprof-card uprof-card--wide">
                    <h3>心理画像</h3>
                    {metricRows(profile.psychology)}
                  </section>
                </div>

                <aside className="uprof-board__side" aria-label="画像辅助信息">
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
                    <h3>下一步推测</h3>
                    {metricRows(profile.predictions)}
                  </section>

                  <section className="uprof-card uprof-card--memory">
                    <h3>长期记忆事件</h3>
                    {memoryItems.length === 0 ? (
                      <p className="uprof-empty">暂无关键事件。</p>
                    ) : (
                      <ul className="uprof-memory-list">
                        {memoryItems.map((item, idx) => (
                          <li key={`${item?.time || 't'}-${idx}`} className="uprof-memory-item">
                            <div className="uprof-memory-item__meta">
                              <span>{String(item?.time || '-')}</span>
                              <span>{String(item?.type || '事件')}</span>
                              <span>重要度 {String(item?.importance ?? '-')}</span>
                            </div>
                            <p>{String(item?.summary || '')}</p>
                          </li>
                        ))}
                      </ul>
                    )}
                  </section>
                </aside>
              </div>
            </>
          )}
        </div>
      </div>
    </div>
  );
}
