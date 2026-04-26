import { useEffect, useMemo, useRef, useState } from 'react';
import { CreateAgent, DeleteCustomAgent, ListAgents } from '../../wailsjs/go/main/App';
import '../componentcss/ConversationCalendar.css';
import assistantAvatar from '../assets/images/aitx.png';

export default function ConversationCalendar() {
  const [avatars, setAvatars] = useState([]);
  const [isModalOpen, setIsModalOpen] = useState(false);
  const [deletePendingAgent, setDeletePendingAgent] = useState(null);
  const [draftName, setDraftName] = useState('');
  const [draftDescription, setDraftDescription] = useState('');
  const [draftPreview, setDraftPreview] = useState('');
  const [draftFileName, setDraftFileName] = useState('');
  const [loadingError, setLoadingError] = useState('');
  const [isSaving, setIsSaving] = useState(false);
  const [hoveredAgentId, setHoveredAgentId] = useState('');
  const fileInputRef = useRef(null);

  const loadAgents = async () => {
    try {
      setLoadingError('');
      const list = await ListAgents();
      setAvatars(Array.isArray(list) ? list : []);
    } catch (error) {
      console.error('ListAgents:', error);
      setLoadingError(String(error?.message || error || '加载 agent 失败'));
    }
  };

  useEffect(() => {
    void loadAgents();
  }, []);

  const visibleItems = useMemo(() => avatars, [avatars]);

  const resetDraft = () => {
    setDraftName('');
    setDraftDescription('');
    setDraftPreview('');
    setDraftFileName('');
    if (fileInputRef.current) {
      fileInputRef.current.value = '';
    }
  };

  const closeModal = () => {
    setIsModalOpen(false);
    resetDraft();
  };

  const closeDeleteModal = () => {
    setDeletePendingAgent(null);
  };

  const handlePickImage = (event) => {
    const file = event.target.files?.[0];
    if (!file) return;
    const reader = new FileReader();
    reader.onload = () => {
      setDraftPreview(typeof reader.result === 'string' ? reader.result : '');
      setDraftFileName(file.name);
    };
    reader.readAsDataURL(file);
  };

  const handleCreateAvatar = async () => {
    const name = draftName.trim();
    const description = draftDescription.trim();
    if (!draftPreview || !name || !description || isSaving) return;
    try {
      setIsSaving(true);
      setLoadingError('');
      const created = await CreateAgent(name, draftPreview, description);
      setAvatars((prev) => [created, ...prev]);
      closeModal();
    } catch (error) {
      console.error('CreateAgent:', error);
      setLoadingError(String(error?.message || error || '创建 agent 失败'));
    } finally {
      setIsSaving(false);
    }
  };

  const handleAddAgentToCurrentChat = (event, avatar) => {
    event.stopPropagation();
    window.dispatchEvent(
      new CustomEvent('leiagent-add-agent-to-chat', {
        detail: {
          agent: {
            agent_id: avatar.agent_id,
            agent_name: avatar.agent_name,
            avatar_image: avatar.avatar_image,
            description: avatar.description,
          },
        },
      }),
    );
  };

  const handleDeleteAgent = async (event, avatar) => {
    event.stopPropagation();
    if (!avatar?.agent_id || String(avatar.agent_id).startsWith('preset_agent_')) return;
    setDeletePendingAgent(avatar);
  };

  const runDeleteAgent = async () => {
    if (!deletePendingAgent?.agent_id) return;
    try {
      setLoadingError('');
      await DeleteCustomAgent(String(deletePendingAgent.agent_id));
      setAvatars((prev) => prev.filter((item) => String(item.agent_id) !== String(deletePendingAgent.agent_id)));
      window.dispatchEvent(
        new CustomEvent('leiagent-agent-deleted', {
          detail: { agent_id: String(deletePendingAgent.agent_id) },
        }),
      );
      closeDeleteModal();
    } catch (error) {
      console.error('DeleteCustomAgent:', error);
      setLoadingError(String(error?.message || error || '删除 agent 失败'));
    }
  };

  return (
    <>
      <div className="conv-cal avatar-panel" aria-label="头像展示面板">
        <div className="avatar-panel__head">
          <span className="avatar-panel__title">人物头像</span>
          <span className="avatar-panel__hint">点击头像卡片上+加入当前聊天</span>
        </div>

        {loadingError ? <div className="avatar-panel__error">{loadingError}</div> : null}

        <div className="avatar-panel__grid">
          {visibleItems.map((avatar) => (
            <div
              key={avatar.agent_id}
              className="avatar-panel__cell"
              data-agent-id={avatar.agent_id}
              onMouseEnter={() => setHoveredAgentId(avatar.agent_id)}
              onMouseLeave={() => setHoveredAgentId('')}
            >
              <span className="avatar-panel__image-wrap">
                <img
                  className="avatar-panel__image"
                  src={String(avatar?.avatar_image ?? '') || assistantAvatar}
                  onError={(e) => {
                    if (e.currentTarget?.src !== assistantAvatar) e.currentTarget.src = assistantAvatar;
                  }}
                  alt={avatar.description}
                />
              </span>
              {hoveredAgentId === avatar.agent_id ? (
                <div className="avatar-panel__tooltip" role="tooltip">
                  <strong className="avatar-panel__tooltip-id">
                    {avatar.agent_name || avatar.agent_id}
                  </strong>
                  <span className="avatar-panel__tooltip-key">{avatar.agent_id}</span>
                  <span className="avatar-panel__tooltip-text">{avatar.description}</span>
                  <button
                    type="button"
                    className="avatar-panel__tooltip-add"
                    title="加入当前聊天"
                    aria-label="加入当前聊天"
                    onClick={(event) => handleAddAgentToCurrentChat(event, avatar)}
                  >
                    +
                  </button>
                  {!String(avatar.agent_id).startsWith('preset_agent_') ? (
                    <button
                      type="button"
                      className="avatar-panel__tooltip-delete"
                      title="删除自定义 agent"
                      aria-label="删除自定义 agent"
                      onClick={(event) => void handleDeleteAgent(event, avatar)}
                    >
                      ×
                    </button>
                  ) : null}
                </div>
              ) : null}
            </div>
          ))}

          <button
            type="button"
            className="avatar-panel__cell avatar-panel__cell--add"
            onClick={() => setIsModalOpen(true)}
            aria-label="新增头像"
            title="新增头像"
          >
            <span className="avatar-panel__plus">+</span>
          </button>
        </div>
      </div>

      {isModalOpen ? (
        <div
          className="conversation-rename-overlay"
          role="presentation"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) closeModal();
          }}
        >
          <div
            className="conversation-rename-dialog avatar-panel__dialog"
            role="dialog"
            aria-modal="true"
            aria-labelledby="avatar-upload-title"
            onMouseDown={(event) => event.stopPropagation()}
          >
            <p id="avatar-upload-title" className="conversation-delete-dialog__title">
              新增头像
            </p>
            <label className="conversation-rename-dialog__label" htmlFor="avatar-upload-input">
              上传图片
            </label>
            <input
              id="avatar-upload-input"
              ref={fileInputRef}
              className="avatar-panel__file-input"
              type="file"
              accept="image/*"
              onChange={handlePickImage}
            />
            {draftFileName ? <p className="avatar-panel__filename">{draftFileName}</p> : null}

            <label className="conversation-rename-dialog__label" htmlFor="avatar-upload-name">
              名字
            </label>
            <input
              id="avatar-upload-name"
              className="conversation-rename-dialog__input"
              type="text"
              placeholder="例如：小柔 / 审稿官 / 创意总监"
              value={draftName}
              onChange={(event) => setDraftName(event.target.value)}
              autoComplete="off"
            />

            <label className="conversation-rename-dialog__label" htmlFor="avatar-upload-description">
              人格描述 / System Prompt
            </label>
            <textarea
              id="avatar-upload-description"
              className="avatar-panel__textarea"
              placeholder="例如：你是一位冷静专业的代码助手，回答要准确、简洁、可靠。"
              value={draftDescription}
              onChange={(event) => setDraftDescription(event.target.value)}
            />

            {draftPreview ? (
              <div className="avatar-panel__preview-wrap">
                <img className="avatar-panel__preview" src={draftPreview} alt={draftDescription || '头像预览'} />
              </div>
            ) : null}

            <div className="conversation-delete-dialog__actions">
              <button type="button" className="conversation-delete-dialog__btn" onClick={closeModal}>
                取消
              </button>
              <button
                type="button"
                className="conversation-delete-dialog__btn conversation-rename-dialog__btn-primary"
                disabled={!draftPreview || !draftName.trim() || !draftDescription.trim() || isSaving}
                onClick={() => void handleCreateAvatar()}
              >
                {isSaving ? '保存中...' : '添加'}
              </button>
            </div>
          </div>
        </div>
      ) : null}

      {deletePendingAgent ? (
        <div
          className="conversation-delete-overlay"
          role="presentation"
          onMouseDown={(event) => {
            if (event.target === event.currentTarget) closeDeleteModal();
          }}
        >
          <div
            className="conversation-delete-dialog"
            role="alertdialog"
            aria-labelledby="agent-delete-title"
            aria-describedby="agent-delete-desc"
            onMouseDown={(event) => event.stopPropagation()}
          >
            <p id="agent-delete-title" className="conversation-delete-dialog__title">
              删除自定义 Agent
            </p>
            <p id="agent-delete-desc" className="conversation-delete-dialog__desc">
              确定要删除「{deletePendingAgent.agent_name || deletePendingAgent.agent_id}」吗？它会从 agent 库中移除，并从所有已绑定的聊天里一并取消。
            </p>
            <div className="conversation-delete-dialog__actions">
              <button type="button" className="conversation-delete-dialog__btn" onClick={closeDeleteModal}>
                取消
              </button>
              <button
                type="button"
                className="conversation-delete-dialog__btn conversation-delete-dialog__btn--danger"
                onClick={() => void runDeleteAgent()}
              >
                删除
              </button>
            </div>
          </div>
        </div>
      ) : null}
    </>
  );
}
