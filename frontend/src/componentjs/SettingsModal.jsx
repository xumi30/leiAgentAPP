import { useCallback, useEffect, useMemo, useRef, useState } from 'react';
import { GetLLMConfigFormState, GetMCPConfigFormState, SaveLLMConfigForm, SaveMCPConfigForm, ValidateMCPConfigRow } from '../../wailsjs/go/main/App';
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

function emptyMcpRow() {
  return {
    label: '',
    transportType: 'stdio',
    url: '',
    command: '',
    argsText: '',
    allowedTools: '',
    headersText: '',
    envText: '',
    cachedTools: [],
  };
}

function emptyMcpStatus() {
  return {
    state: 'idle',
    message: '',
    tools: [],
    toolCount: 0,
    checkedAt: '',
  };
}

function parseMcpImportText(rawText) {
  const text = String(rawText ?? '').trim();
  if (!text) {
    throw new Error('请先粘贴 MCP JSON 配置');
  }
  let parsed;
  try {
    parsed = JSON.parse(text);
  } catch (e) {
    throw new Error(`JSON 解析失败：${String(e?.message || e)}`);
  }

  const source =
    parsed && typeof parsed === 'object' && parsed.mcpServers && typeof parsed.mcpServers === 'object'
      ? parsed.mcpServers
      : parsed;

  if (!source || typeof source !== 'object' || Array.isArray(source)) {
    throw new Error('未找到可解析的 mcpServers 对象');
  }

  const rows = Object.entries(source).flatMap(([label, cfg]) => {
    if (!cfg || typeof cfg !== 'object' || Array.isArray(cfg)) {
      return [];
    }
    const command = String(cfg.command ?? '').trim();
    const url = String(cfg.url ?? cfg.server_url ?? '').trim();
    const args = Array.isArray(cfg.args) ? cfg.args.map((v) => String(v)) : [];
    const headers = cfg.headers && typeof cfg.headers === 'object' && !Array.isArray(cfg.headers) ? cfg.headers : {};
    const env = cfg.env && typeof cfg.env === 'object' && !Array.isArray(cfg.env) ? cfg.env : {};
    const allowedTools = Array.isArray(cfg.allowed_tools)
      ? cfg.allowed_tools
      : Array.isArray(cfg.allowedTools)
        ? cfg.allowedTools
        : [];

    return [
      {
        ...emptyMcpRow(),
        label: String(label ?? '').trim(),
        transportType: String(cfg.transport_type ?? cfg.transportType ?? (command ? 'stdio' : 'streamable_http')).trim(),
        url,
        command,
        argsText: args.join('\n'),
        allowedTools: allowedTools.map((v) => String(v)).join('\n'),
        headersText: Object.entries(headers)
          .map(([k, v]) => `${k}: ${String(v)}`)
          .join('\n'),
        envText: Object.entries(env)
          .map(([k, v]) => `${k}: ${String(v)}`)
          .join('\n'),
        cachedTools: [],
      },
    ];
  });

  if (rows.length === 0) {
    throw new Error('没有解析出任何 MCP 服务');
  }
  return rows;
}

function isMcpRowReady(row) {
  const label = String(row?.label ?? '').trim();
  const url = String(row?.url ?? '').trim();
  const command = String(row?.command ?? '').trim();
  return label !== '' && (url !== '' || command !== '');
}

function sameToolList(a, b) {
  const aa = Array.isArray(a) ? a : [];
  const bb = Array.isArray(b) ? b : [];
  if (aa.length !== bb.length) return false;
  for (let i = 0; i < aa.length; i += 1) {
    if (aa[i] !== bb[i]) return false;
  }
  return true;
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
  const [activeTab, setActiveTab] = useState('llm');
  const [backends, setBackends] = useState(() => []);
  const [mcpServers, setMcpServers] = useState(() => []);
  const [mcpStatuses, setMcpStatuses] = useState(() => []);
  const [mcpImportText, setMcpImportText] = useState('');
  const [savePath, setSavePath] = useState('');
  const [usingExample, setUsingExample] = useState(false);
  const [loadErr, setLoadErr] = useState('');
  const [saveErr, setSaveErr] = useState('');
  const [saving, setSaving] = useState(false);
  const lastValidatedRef = useRef([]);

  const load = useCallback(async () => {
    setLoadErr('');
    try {
      const [llmState, mcpState] = await Promise.all([GetLLMConfigFormState(), GetMCPConfigFormState()]);
      const llmList = Array.isArray(llmState.backends) ? llmState.backends : [];
      const mcpList = Array.isArray(mcpState.servers) ? mcpState.servers : [];
      setBackends(llmList.length > 0 ? llmList.map(mapBackendRow) : []);
      const nextMcp = mcpList.length > 0 ? mcpList.map((row) => ({ ...emptyMcpRow(), ...row })) : [];
      setMcpServers(nextMcp);
      setMcpStatuses(
        nextMcp.map((row) =>
          row.lastCheckState
            ? {
                state: row.lastCheckState,
                message: row.lastCheckMessage || (Array.isArray(row.cachedTools) && row.cachedTools.length > 0 ? `已缓存 ${row.cachedTools.length} 个工具` : ''),
                tools: Array.isArray(row.cachedTools) ? row.cachedTools : [],
                toolCount: Array.isArray(row.cachedTools) ? row.cachedTools.length : 0,
                checkedAt: row.lastCheckedAt || '',
              }
            : emptyMcpStatus()
        )
      );
      lastValidatedRef.current = nextMcp.map(() => '');
      setSavePath(llmState.path ?? mcpState.path ?? '');
      setUsingExample(!!(llmState.usingExample || mcpState.usingExample));
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

  const updateMcpServer = (index, field, value) => {
    setMcpServers((prev) => {
      const next = [...prev];
      next[index] = { ...next[index], [field]: value, ...(field === 'cachedTools' ? {} : { cachedTools: [] }) };
      return next;
    });
  };

  const addMcpServerRow = () => {
    setMcpServers((prev) => [...prev, emptyMcpRow()]);
    setMcpStatuses((prev) => [...prev, emptyMcpStatus()]);
    lastValidatedRef.current = [...lastValidatedRef.current, ''];
  };

  const removeMcpServerRow = (index) => {
    setMcpServers((prev) => prev.filter((_, i) => i !== index));
    setMcpStatuses((prev) => prev.filter((_, i) => i !== index));
    lastValidatedRef.current = lastValidatedRef.current.filter((_, i) => i !== index);
  };

  const handleImportMcp = () => {
    setSaveErr('');
    try {
      const rows = parseMcpImportText(mcpImportText);
      setMcpServers((prev) => [...prev, ...rows]);
      setMcpStatuses((prev) => [...prev, ...rows.map(() => emptyMcpStatus())]);
      lastValidatedRef.current = [...lastValidatedRef.current, ...rows.map(() => '')];
      setMcpImportText('');
    } catch (e) {
      setSaveErr(String(e?.message || e));
    }
  };

  const setMcpStatusAt = useCallback((index, next) => {
    setMcpStatuses((prev) => {
      const out = [...prev];
      out[index] = { ...emptyMcpStatus(), ...(out[index] ?? {}), ...next };
      return out;
    });
  }, []);

  const validateMcpRow = useCallback(async (index, row) => {
    if (!isMcpRowReady(row)) {
      setMcpStatusAt(index, emptyMcpStatus());
      return null;
    }
    setMcpStatusAt(index, { state: 'checking', message: '校验中…' });
    try {
      const result = await ValidateMCPConfigRow(row);
      const tools = Array.isArray(result?.tools) ? result.tools : [];
        setMcpStatusAt(index, {
        state: result?.lastCheckState || (result?.ok ? 'ok' : 'error'),
        message: String(result?.message ?? ''),
        tools,
        toolCount: typeof result?.toolCount === 'number' ? result.toolCount : tools.length,
        checkedAt: result?.checkedAt ?? '',
      });
      if (result?.ok) {
        setMcpServers((prev) => {
          const next = [...prev];
          const current = next[index];
          if (!current) {
            return prev;
          }
          const nextDetails = Array.isArray(result?.toolDetails) ? result.toolDetails : [];
          if (sameToolList(current.cachedTools, tools) && current.lastCheckState === 'ok' && current.lastCheckMessage === String(result?.message ?? '') && current.lastCheckedAt === (result?.checkedAt ?? '')) {
            return prev;
          }
          next[index] = {
            ...current,
            cachedTools: tools,
            cachedToolDetails: nextDetails,
            lastCheckState: result?.lastCheckState || 'ok',
            lastCheckMessage: String(result?.message ?? ''),
            lastCheckedAt: result?.checkedAt ?? '',
          };
          return next;
        });
      } else {
        setMcpServers((prev) => {
          const next = [...prev];
          const current = next[index];
          if (!current) {
            return prev;
          }
          next[index] = {
            ...current,
            cachedTools: [],
            cachedToolDetails: [],
            lastCheckState: result?.lastCheckState || 'error',
            lastCheckMessage: String(result?.message ?? ''),
            lastCheckedAt: result?.checkedAt ?? '',
          };
          return next;
        });
      }
      return result;
    } catch (e) {
      const message = String(e?.message || e);
      setMcpStatusAt(index, { state: 'error', message, tools: [], toolCount: 0, checkedAt: '' });
      return { ok: false, message, tools: [], toolCount: 0 };
    }
  }, [setMcpStatusAt]);

  const validateAllMcpServers = useCallback(async () => {
    const nextRows = [...mcpServers];
    for (let i = 0; i < nextRows.length; i += 1) {
      const row = nextRows[i];
      if (!isMcpRowReady(row)) continue;
      const result = await validateMcpRow(i, row);
      if (result?.ok && Array.isArray(result.tools)) {
        nextRows[i] = { ...nextRows[i], cachedTools: result.tools };
      }
    }
    return nextRows;
  }, [mcpServers, validateMcpRow]);

  const mcpSignatures = useMemo(
    () =>
      mcpServers.map((row) =>
        JSON.stringify({
          label: row.label,
          transportType: row.transportType,
          url: row.url,
          command: row.command,
          argsText: row.argsText,
          allowedTools: row.allowedTools,
          headersText: row.headersText,
          envText: row.envText,
        })
      ),
    [mcpServers]
  );

  useEffect(() => {
    if (!open || activeTab !== 'mcp') return undefined;
    const timer = setTimeout(() => {
      mcpServers.forEach((row, index) => {
        const sig = mcpSignatures[index] ?? '';
        if (!isMcpRowReady(row)) {
          lastValidatedRef.current[index] = '';
          setMcpStatusAt(index, emptyMcpStatus());
          return;
        }
        if (lastValidatedRef.current[index] === sig) {
          return;
        }
        lastValidatedRef.current[index] = sig;
        void validateMcpRow(index, row);
      });
    }, 700);
    return () => clearTimeout(timer);
  }, [activeTab, mcpServers, mcpSignatures, open, setMcpStatusAt, validateMcpRow]);

  const handleSave = async () => {
    setSaveErr('');
    setSaving(true);
    try {
      if (activeTab === 'mcp') {
        const validatedRows = await validateAllMcpServers();
        await SaveMCPConfigForm(validatedRows);
      } else {
        await SaveLLMConfigForm({}, backends);
      }
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

  const statusClassName = (state) => `settings-status-dot settings-status-dot--${state || 'idle'}`;

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
            设置
          </h2>
          <button type="button" className="settings-sheet__close" onClick={onClose} aria-label="关闭">
            完成
          </button>
        </div>

        <div className="settings-tabs" role="tablist" aria-label="设置分组">
          <button
            type="button"
            className={`settings-tabs__btn ${activeTab === 'llm' ? 'settings-tabs__btn--active' : ''}`}
            onClick={() => setActiveTab('llm')}
          >
            LLM
          </button>
          <button
            type="button"
            className={`settings-tabs__btn ${activeTab === 'mcp' ? 'settings-tabs__btn--active' : ''}`}
            onClick={() => setActiveTab('mcp')}
          >
            MCP
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
          {activeTab === 'mcp'
            ? '每行对应一个 MCP 服务。stdio 模式填写 command 与 args；HTTP 模式填写 url。多值字段按行输入，headers/env 使用 key: value。'
            : '按顺序故障转移；仅勾选启用的行会参与。每行须填写 base_url、model。某行未填 API Key 时，若文件中 llm.api_key 已填写会回退使用该 Key，否则用环境变量。'}
        </p>

        {loadErr ? <div className="settings-sheet__error">{loadErr}</div> : null}
        {saveErr ? <div className="settings-sheet__error">{saveErr}</div> : null}

        {activeTab === 'llm' ? (
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
        ) : (
          <div className="settings-sheet__scroll">
            <div className="settings-list-block">
              <div className="settings-list-block__toolbar">
                <div className="settings-import">
                  <textarea
                    className="settings-import__textarea"
                    value={mcpImportText}
                    onChange={(e) => setMcpImportText(e.target.value)}
                    placeholder={'粘贴 mcpServers JSON，例如：{\n  "mcpServers": {\n    "novel-workflow": {\n      "command": "npx",\n      "args": ["-y", "@ttaqt/novel-workflow-mcp@latest"]\n    }\n  }\n}'}
                    spellCheck={false}
                  />
                  <div className="settings-import__actions">
                    <button type="button" className="settings-btn settings-btn--secondary settings-btn--small" onClick={addMcpServerRow}>
                      添加服务
                    </button>
                    <button type="button" className="settings-btn settings-btn--primary settings-btn--small" onClick={handleImportMcp}>
                      粘贴解析
                    </button>
                  </div>
                </div>
              </div>
              <table className="settings-table settings-table--wide settings-table--mcp">
                <thead>
                  <tr>
                    <th className="settings-table__th-status">状态</th>
                    <th>Label</th>
                    <th>Transport</th>
                    <th>URL</th>
                    <th>Command</th>
                    <th>Args</th>
                    <th>Allowed Tools</th>
                    <th>Headers</th>
                    <th>Env</th>
                    <th className="settings-table__th-actions" />
                  </tr>
                </thead>
                <tbody>
                  {mcpServers.length === 0 ? (
                    <tr>
                      <td colSpan={9} className="settings-table__empty">
                        暂无 MCP 服务，点击「添加服务」开始配置。
                      </td>
                    </tr>
                  ) : (
                    mcpServers.map((row, i) => (
                      <tr key={i}>
                        <td className="settings-table__status-cell">
                          <span className={statusClassName(mcpStatuses[i]?.state)} title={mcpStatuses[i]?.message || '未校验'} />
                        </td>
                        <td>
                          <input className="settings-table__input" value={row.label} onChange={(e) => updateMcpServer(i, 'label', e.target.value)} placeholder="chrome-devtools" spellCheck={false} autoComplete="off" />
                          <div className="settings-mcp-meta">
                            <div className="settings-mcp-meta__text">{mcpStatuses[i]?.message || '等待配置完成后自动校验'}</div>
                            {Array.isArray(
                              mcpStatuses[i]?.state === 'ok'
                                ? mcpStatuses[i]?.tools
                                : mcpStatuses[i]?.state === 'idle'
                                  ? row.cachedTools
                                  : []
                            ) &&
                            (mcpStatuses[i]?.state === 'ok' ? mcpStatuses[i]?.tools?.length : mcpStatuses[i]?.state === 'idle' ? row.cachedTools?.length : 0) ? (
                              <div className="settings-mcp-tools">
                                {(mcpStatuses[i]?.state === 'ok' ? mcpStatuses[i]?.tools : row.cachedTools).slice(0, 6).map((tool) => (
                                  <span key={tool} className="settings-mcp-tool-chip">
                                    {tool}
                                  </span>
                                ))}
                                {(mcpStatuses[i]?.state === 'ok' ? mcpStatuses[i]?.toolCount : row.cachedTools?.length || 0) > 6 ? (
                                  <span className="settings-mcp-tool-chip settings-mcp-tool-chip--muted">+更多</span>
                                ) : null}
                              </div>
                            ) : null}
                          </div>
                        </td>
                        <td>
                          <input className="settings-table__input" value={row.transportType} onChange={(e) => updateMcpServer(i, 'transportType', e.target.value)} placeholder="stdio | streamable_http" spellCheck={false} autoComplete="off" />
                        </td>
                        <td>
                          <input className="settings-table__input" value={row.url} onChange={(e) => updateMcpServer(i, 'url', e.target.value)} placeholder="http://127.0.0.1:3001" spellCheck={false} autoComplete="off" />
                        </td>
                        <td>
                          <input className="settings-table__input" value={row.command} onChange={(e) => updateMcpServer(i, 'command', e.target.value)} placeholder="/abs/path/to/bin 或 npx" spellCheck={false} autoComplete="off" />
                        </td>
                        <td>
                          <textarea className="settings-table__textarea" value={row.argsText} onChange={(e) => updateMcpServer(i, 'argsText', e.target.value)} placeholder="每行一个参数" spellCheck={false} />
                        </td>
                        <td>
                          <textarea className="settings-table__textarea" value={row.allowedTools} onChange={(e) => updateMcpServer(i, 'allowedTools', e.target.value)} placeholder="每行一个工具名" spellCheck={false} />
                        </td>
                        <td>
                          <textarea className="settings-table__textarea" value={row.headersText} onChange={(e) => updateMcpServer(i, 'headersText', e.target.value)} placeholder="Authorization: Bearer xxx" spellCheck={false} />
                        </td>
                        <td>
                          <textarea className="settings-table__textarea" value={row.envText} onChange={(e) => updateMcpServer(i, 'envText', e.target.value)} placeholder="KEY: value" spellCheck={false} />
                        </td>
                        <td className="settings-table__actions">
                          <button
                            type="button"
                            className="settings-table__validate"
                            onClick={() => validateMcpRow(i, row)}
                            aria-label="校验此服务"
                          >
                            校验
                          </button>
                          <button
                            type="button"
                            className="settings-table__remove"
                            onClick={() => removeMcpServerRow(i)}
                            aria-label="删除此服务"
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
        )}

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
