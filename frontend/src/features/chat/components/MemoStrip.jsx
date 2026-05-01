import React from 'react';

/**
 * MemoStrip（生成备忘录区域）
 * - UI only，所有逻辑由 `useMemoComposer` 提供
 *
 * @param {{
 *  open: boolean,
 *  busy: boolean,
 *  markedCount: number,
 *  presets: { id: string, label: string, text: string }[],
 *  presetAddOpen: boolean,
 *  draftLabel: string,
 *  draftText: string,
 *  composeHint: string,
 *  onToggleOpen: () => void,
 *  onSetComposeHint: (v: string) => void,
 *  onSaveDirect: () => void,
 *  onSendLLM: () => void,
 *  onTogglePresetAdd: () => void,
 *  onDraftLabel: (v: string) => void,
 *  onDraftText: (v: string) => void,
 *  onAddPreset: () => void,
 *  onRemovePreset: (id: string) => void,
 * }} props
 */
export default function MemoStrip({
  open,
  busy,
  markedCount,
  presets,
  presetAddOpen,
  draftLabel,
  draftText,
  composeHint,
  onToggleOpen,
  onSetComposeHint,
  onSaveDirect,
  onSendLLM,
  onTogglePresetAdd,
  onDraftLabel,
  onDraftText,
  onAddPreset,
  onRemovePreset,
}) {
  const list = Array.isArray(presets) ? presets : [];

  return (
    <div className="dialog__memo-strip">
      <div className="dialog__memo-toolbar" role="toolbar" aria-label="输入区快捷操作">
        <button
          type="button"
          className="dialog__memo-pill"
          disabled={busy}
          onClick={onToggleOpen}
          aria-expanded={open}
          title={open ? '退出勾选模式，取消本次生成备忘' : '在消息旁勾选要收录的内容'}
        >
          {open ? '取消生成' : '生成备忘'}
        </button>
      </div>

      {open ? (
        <div className={`dialog__memo-compose${busy ? ' dialog__memo-compose--busy' : ''}`}>
          <p className="dialog__memo-compose-lead">
            在每条消息旁勾选（可多选）。标题优先取<strong>用户</strong>消息首行，否则助手。
          </p>

          {markedCount > 0 ? (
            <>
              <div className="dialog__memo-preset-bar" role="group" aria-label="快捷提示词，点击填入下方输入框">
                <span className="dialog__memo-preset-bar__label">快捷</span>
                {list.map((p) => {
                  const isBuiltin = String(p.id).startsWith('builtin:');
                  return (
                    <span key={p.id} className="dialog__memo-preset-chip-wrap">
                      <button
                        type="button"
                        className="dialog__memo-preset-chip"
                        disabled={busy}
                        title={p.text}
                        onClick={() => onSetComposeHint(p.text)}
                      >
                        {p.label}
                      </button>
                      {!isBuiltin ? (
                        <button
                          type="button"
                          className="dialog__memo-preset-chip-del"
                          aria-label={`删除快捷「${p.label}」`}
                          disabled={busy}
                          onClick={(e) => {
                            e.preventDefault();
                            onRemovePreset(p.id);
                          }}
                        >
                          ×
                        </button>
                      ) : null}
                    </span>
                  );
                })}
                <button
                  type="button"
                  className="dialog__memo-preset-add"
                  disabled={busy}
                  title="添加自定义快捷提示词"
                  aria-expanded={presetAddOpen}
                  aria-label="添加自定义快捷提示词"
                  onClick={onTogglePresetAdd}
                >
                  +
                </button>
              </div>

              {presetAddOpen ? (
                <div className="dialog__memo-preset-editor">
                  <div className="dialog__memo-preset-editor__row">
                    <input
                      type="text"
                      className="dialog__memo-preset-editor__label"
                      placeholder="显示名称（短，如：严肃总结）"
                      value={draftLabel}
                      onChange={(e) => onDraftLabel(e.target.value)}
                      maxLength={24}
                      autoComplete="off"
                    />
                    <textarea
                      className="dialog__memo-preset-editor__text"
                      rows={2}
                      placeholder="点击标签后填入的完整提示词…"
                      value={draftText}
                      onChange={(e) => onDraftText(e.target.value)}
                      maxLength={800}
                    />
                  </div>
                  <div className="dialog__memo-preset-editor__actions">
                    <button
                      type="button"
                      className="dialog__memo-preset-editor__btn dialog__memo-preset-editor__btn--primary"
                      onClick={onAddPreset}
                    >
                      添加
                    </button>
                    <button
                      type="button"
                      className="dialog__memo-preset-editor__btn"
                      onClick={onTogglePresetAdd}
                    >
                      取消
                    </button>
                  </div>
                </div>
              ) : null}

              <textarea
                className="dialog__memo-compose-input"
                rows={2}
                placeholder="可选：写给模型的整理要求（语气、侧重点、长度等）…"
                value={composeHint}
                onChange={(e) => onSetComposeHint(e.target.value)}
                disabled={busy}
              />
              <div className="dialog__memo-actions">
                <button
                  type="button"
                  className="dialog__memo-write-btn dialog__memo-write-btn--secondary"
                  disabled={busy}
                  onClick={onSaveDirect}
                >
                  直接写入
                </button>
                <button
                  type="button"
                  className="dialog__memo-write-btn"
                  disabled={busy}
                  onClick={onSendLLM}
                >
                  {busy ? '处理中…' : '发送（模型优化）'}
                </button>
              </div>
            </>
          ) : null}
        </div>
      ) : null}
    </div>
  );
}
