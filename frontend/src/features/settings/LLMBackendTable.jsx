import { isProxyLbDisplayRow, proxyLbApiKeyFieldValue } from './settingsModel';

export default function LLMBackendTable({
  backends,
  onAdd,
  onRemove,
  onUpdate,
  proxyLbHasSession,
  proxyLbAuthFailed,
}) {
  const colInputs = (row, index, lbVariant) => {
    const locked = isProxyLbDisplayRow(row);
    const ti = locked ? -1 : 0;
    const cell = (child) =>
      locked ? (
        <div className={`settings-table__proxy-lb-cell settings-table__proxy-lb-cell--${lbVariant}`}>{child}</div>
      ) : (
        child
      );
    return (
    <>
      <td className="settings-table__cell-check">
        {cell(
          <input
            type="checkbox"
            className="settings-table__check"
            checked={row.enabled}
            onChange={(e) => onUpdate(index, 'enabled', e.target.checked)}
            disabled={locked}
            tabIndex={ti}
            title={locked ? 'Proxy-LB 行为登录会话管理' : '启用后参与故障转移'}
            aria-label="启用此后端"
          />,
        )}
      </td>
      <td>
        {cell(
          <input
            className="settings-table__input"
            value={row.name}
            onChange={(e) => onUpdate(index, 'name', e.target.value)}
            readOnly={locked}
            tabIndex={ti}
            placeholder="标识"
            spellCheck={false}
            autoComplete="off"
          />,
        )}
      </td>
      <td>
        {cell(
          <input
            className="settings-table__input"
            type={locked && String(row.apiKey ?? '').trim() ? 'text' : 'password'}
            value={proxyLbApiKeyFieldValue(row, locked)}
            onChange={(e) => onUpdate(index, 'apiKey', e.target.value)}
            readOnly={locked}
            tabIndex={ti}
            placeholder="api_key"
            spellCheck={false}
            autoComplete="off"
          />,
        )}
      </td>
      <td>
        {cell(
          <input
            className="settings-table__input"
            value={row.baseUrl}
            onChange={(e) => onUpdate(index, 'baseUrl', e.target.value)}
            readOnly={locked}
            tabIndex={ti}
            placeholder="base_url"
            spellCheck={false}
            autoComplete="off"
          />,
        )}
      </td>
      <td>
        {cell(
          <input
            className="settings-table__input"
            value={row.model}
            onChange={(e) => onUpdate(index, 'model', e.target.value)}
            readOnly={locked}
            tabIndex={ti}
            placeholder="model"
            spellCheck={false}
            autoComplete="off"
          />,
        )}
      </td>
      <td>
        {cell(
          <input
            className="settings-table__input"
            value={row.provider}
            onChange={(e) => onUpdate(index, 'provider', e.target.value)}
            readOnly={locked}
            tabIndex={ti}
            placeholder="留空 / gemini"
            spellCheck={false}
            autoComplete="off"
          />,
        )}
      </td>
      <td>
        {cell(
          <input
            className="settings-table__input"
            value={row.streamMode}
            onChange={(e) => onUpdate(index, 'streamMode', e.target.value)}
            readOnly={locked}
            tabIndex={ti}
            placeholder="nonstream | stream | both"
            spellCheck={false}
            autoComplete="off"
          />,
        )}
      </td>
      <td>
        {cell(
          <input
            className="settings-table__input settings-table__input--num"
            type="number"
            min={0}
            readOnly={locked}
            tabIndex={ti}
            value={row.maxOutputTokens || ''}
            onChange={(e) => {
              const v = e.target.value;
              onUpdate(index, 'maxOutputTokens', v === '' ? 0 : parseInt(v, 10) || 0);
            }}
            placeholder="0=默认"
          />,
        )}
      </td>
    </>
    );
  };

  return (
          <div className="settings-sheet__scroll">
            <div className="settings-list-block">
              <div className="settings-list-block__toolbar">
                <button type="button" className="settings-btn settings-btn--secondary settings-btn--small" onClick={onAdd}>添加一行</button>
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
                    <tr><td colSpan={9} className="settings-table__empty">暂无行，点击「添加一行」配置多后端。</td></tr>
                  ) : (
                    backends.map((row, i) => {
                      const locked = isProxyLbDisplayRow(row);
                      const lbVariant = locked
                        ? (proxyLbHasSession ? 'ok' : proxyLbAuthFailed ? 'fail' : 'idle')
                        : null;
                      return (
                      <tr
                        key={i}
                        className={
                          locked
                            ? `settings-table__tr--proxy-lb-readonly settings-table__tr--proxy-lb--${lbVariant}`
                            : undefined
                        }
                      >
                        {colInputs(row, i, lbVariant)}
                        <td className="settings-table__actions">
                          {locked ? (
                            <div className={`settings-table__proxy-lb-cell settings-table__proxy-lb-cell--${lbVariant}`}>
                              <button
                                type="button"
                                className="settings-table__remove"
                                disabled
                                tabIndex={-1}
                                title="Proxy-LB 行请在 PROXYLB 中登出以清除"
                                aria-label="此行不可删除"
                              >
                                删除
                              </button>
                            </div>
                          ) : (
                            <button type="button" className="settings-table__remove" onClick={() => onRemove(i)} aria-label="删除此行">删除</button>
                          )}
                        </td>
                      </tr>
                      );
                    })
                  )}
                </tbody>
              </table>
            </div>
          </div>
  );
}
