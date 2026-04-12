import { useCallback, useEffect, useState } from 'react';
import { GetLLMConfigFormState, SaveLLMConfigForm } from '../../wailsjs/go/main/App';
import '../componentcss/SettingsModal.css';

function emptyRow() {
  return {
    name: '',
    apiKey: '',
    baseUrl: '',
    model: '',
    provider: '',
    streamMode: 'both',
    maxOutputTokens: 0,
    enabled: true,
  };
}

/** 与 mergeLLMYAML 一致：未配置时非 gemini 为 both，gemini 为 nonstream */
function effectiveStreamMode(raw, provider) {
  const s = String(raw ?? '').trim();
  if (s) return s;
  return String(provider ?? '').toLowerCase().trim() === 'gemini' ? 'nonstream' : 'both';
}

function mapBackendRow(r) {
  return {
    name: r.name ?? '',
    apiKey: r.apiKey ?? r.api_key ?? '',
    baseUrl: r.baseUrl ?? r.base_url ?? '',
    model: r.model ?? '',
    provider: r.provider ?? '',
    streamMode: effectiveStreamMode(r.streamMode ?? r.stream_mode, r.provider),
    maxOutputTokens:
      typeof r.maxOutputTokens === 'number'
        ? r.maxOutputTokens
        : typeof r.max_output_tokens === 'number'
          ? r.max_output_tokens
          : 0,
    enabled: r.enabled !== false,
  };
}

export default function SettingsModal({ open, onClose, onSaved }) {
  const [backends, setBackends] = useState(() => []);
  const [savePath, setSavePath] = useState('');
  const [usingExample, setUsingExample] = useState(false);
  const [loadErr, setLoadErr] = useState('');
  const [saveErr, setSaveErr] = useState('');
  const [saving, setSaving] = useState(false);

  const load = useCallback(async () => {
    setLoadErr('');
    try {
      const state = await GetLLMConfigFormState();
      const list = Array.isArray(state.backends) ? state.backends : [];
      setBackends(list.length > 0 ? list.map(mapBackendRow) : []);
      setSavePath(state.path ?? '');
      setUsingExample(!!state.usingExample);
    } catch (e) {
      setLoadErr(String(e?.message || e));
    }
  }, []);

  useEffect(() => {
    if (open) {
      load();
    }
  }, [open, load]);

  const updateBackend = (index, field, value) => {
    setBackends((prev) => {
      const next = [...prev];
      next[index] = { ...next[index], [field]: value };
      return next;
    });
  };

  const addBackendRow = () => {
    setBackends((prev) => [...prev, emptyRow()]);
  };

  const removeBackendRow = (index) => {
    setBackends((prev) => prev.filter((_, i) => i !== index));
  };

  const handleSave = async () => {
    setSaveErr('');
    setSaving(true);
    try {
      await SaveLLMConfigForm({}, backends);
      setUsingExample(false);
      onSaved?.();
      onClose();
    } catch (e) {
      setSaveErr(String(e?.message || e));
    } finally {
      setSaving(false);
    }
  };

  if (!open) return null;

  const colInputs = (row, index) => (
    <>
      <td className="settings-table__cell-check">
        <input
          type="checkbox"
          className="settings-table__check"
          checked={row.enabled}
          onChange={(e) => updateBackend(index, 'enabled', e.target.checked)}
          title="启用后参与故障转移"
          aria-label="启用此后端"
        />
      </td>
      <td>
        <input
          className="settings-table__input"
          value={row.name}
          onChange={(e) => updateBackend(index, 'name', e.target.value)}
          placeholder="标识"
          spellCheck={false}
          autoComplete="off"
        />
      </td>
      <td>
        <input
          className="settings-table__input"
          type="password"
          value={row.apiKey}
          onChange={(e) => updateBackend(index, 'apiKey', e.target.value)}
          placeholder="api_key"
          spellCheck={false}
          autoComplete="off"
        />
      </td>
      <td>
        <input
          className="settings-table__input"
          value={row.baseUrl}
          onChange={(e) => updateBackend(index, 'baseUrl', e.target.value)}
          placeholder="base_url"
          spellCheck={false}
          autoComplete="off"
        />
      </td>
      <td>
        <input
          className="settings-table__input"
          value={row.model}
          onChange={(e) => updateBackend(index, 'model', e.target.value)}
          placeholder="model"
          spellCheck={false}
          autoComplete="off"
        />
      </td>
      <td>
        <input
          className="settings-table__input"
          value={row.provider}
          onChange={(e) => updateBackend(index, 'provider', e.target.value)}
          placeholder="留空 / gemini"
          spellCheck={false}
          autoComplete="off"
        />
      </td>
      <td>
        <input
          className="settings-table__input"
          value={row.streamMode}
          onChange={(e) => updateBackend(index, 'streamMode', e.target.value)}
          placeholder="nonstream | stream | both"
          spellCheck={false}
          autoComplete="off"
        />
      </td>
      <td>
        <input
          className="settings-table__input settings-table__input--num"
          type="number"
          min={0}
          value={row.maxOutputTokens || ''}
          onChange={(e) => {
            const v = e.target.value;
            updateBackend(index, 'maxOutputTokens', v === '' ? 0 : parseInt(v, 10) || 0);
          }}
          placeholder="0=默认"
        />
      </td>
    </>
  );

  return (
    <div className="settings-overlay" role="presentation" onMouseDown={onClose}>
      <div
        className="settings-sheet settings-sheet--wide"
        role="dialog"
        aria-labelledby="settings-title"
        onMouseDown={(e) => e.stopPropagation()}
      >
        <div className="settings-sheet__header">
          <h2 id="settings-title" className="settings-sheet__title">
            模型与连接
          </h2>
          <button type="button" className="settings-sheet__close" onClick={onClose} aria-label="关闭">
            完成
          </button>
        </div>

        <p className="settings-sheet__path">
          {savePath ? (
            <>
              <span className="settings-sheet__path-label">保存路径</span>
              <code className="settings-sheet__path-value">{savePath}</code>
            </>
          ) : null}
          {usingExample ? (
            <span className="settings-sheet__hint">尚未有配置文件，保存后将创建 config.yaml</span>
          ) : null}
        </p>

        <p className="settings-sheet__note">
          按顺序故障转移；仅<strong>勾选启用</strong>的行会参与。每行须填写 base_url、model。某行未填 API Key 时，若文件中
          llm.api_key 已填写会回退使用该 Key，否则用环境变量。
        </p>

        {loadErr ? <div className="settings-sheet__error">{loadErr}</div> : null}
        {saveErr ? <div className="settings-sheet__error">{saveErr}</div> : null}

        <div className="settings-sheet__scroll">
          <div className="settings-list-block">
            <div className="settings-list-block__toolbar">
              <button type="button" className="settings-btn settings-btn--secondary settings-btn--small" onClick={addBackendRow}>
                添加一行
              </button>
            </div>
            <table className="settings-table settings-table--wide">
              <thead>
                <tr>
                  <th className="settings-table__th-check">启用</th>
                  <th>名称</th>
                  <th>API Key</th>
                  <th>Base URL</th>
                  <th>Model</th>
                  <th>Provider</th>
                  <th>Stream</th>
                  <th>Max tokens</th>
                  <th className="settings-table__th-actions" />
                </tr>
              </thead>
              <tbody>
                {backends.length === 0 ? (
                  <tr>
                    <td colSpan={9} className="settings-table__empty">
                      暂无行，点击「添加一行」配置多后端。
                    </td>
                  </tr>
                ) : (
                  backends.map((row, i) => (
                    <tr key={i}>
                      {colInputs(row, i)}
                      <td className="settings-table__actions">
                        <button
                          type="button"
                          className="settings-table__remove"
                          onClick={() => removeBackendRow(i)}
                          aria-label="删除此行"
                        >
                          删除
                        </button>
                      </td>
                    </tr>
                  ))
                )}
              </tbody>
            </table>
          </div>
        </div>

        <div className="settings-sheet__actions">
          <button type="button" className="settings-btn settings-btn--secondary" onClick={onClose}>
            取消
          </button>
          <button
            type="button"
            className="settings-btn settings-btn--primary"
            onClick={handleSave}
            disabled={saving}
          >
            {saving ? '保存中…' : '保存并校验'}
          </button>
        </div>
      </div>
    </div>
  );
}
