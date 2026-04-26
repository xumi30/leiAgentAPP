import React from 'react';
import { MAIN_SHEET_ID } from '../constants';
import AgentChip from './AgentChip';

/**
 * TabHeader
 * - 仅负责 UI：tabs + agents chips
 *
 * @param {{
 *  sortedSheets: { id: string, title: string, startIdx: number }[],
 *  activeSheetId: string,
 *  conversationTitle: string,
 *  conversationTokenTotal: number,
 *  listMacaron?: { bg: string, text: string },
 *  currentChatAgents: any[],
 * }} props
 */
export default function TabHeader({
  sortedSheets,
  activeSheetId,
  conversationTitle,
  conversationTokenTotal,
  listMacaron,
  currentChatAgents,
}) {
  const sheets = Array.isArray(sortedSheets) ? sortedSheets : [];
  const agents = Array.isArray(currentChatAgents) ? currentChatAgents : [];

  return (
    <div className="dialog__header dialog__header--tabs">
      <div className="dialog__tabs" role="tablist" aria-label="同一会话便签页">
        {sheets.map((s) => (
          <button
            key={s.id}
            type="button"
            role="tab"
            aria-selected={activeSheetId === s.id}
            className={
              'dialog__tab'
              + (activeSheetId === s.id ? ' dialog__tab--active' : '')
              + (s.id === MAIN_SHEET_ID ? ' dialog__tab--main dialog__tab--convo-tint' : '')
            }
            style={s.id === MAIN_SHEET_ID ? { backgroundColor: listMacaron?.bg, color: listMacaron?.text } : undefined}
            title={
              s.id === MAIN_SHEET_ID
                ? `${(conversationTitle || s.title || '主对话').trim() || '主对话'} · ${Number(conversationTokenTotal || 0).toLocaleString()} tokens`
                : s.title
            }
          >
            {s.id === MAIN_SHEET_ID ? (
              <span className="dialog__tab-inline" dir="auto">
                <span className="dialog__tab-main-title-inline">
                  {(conversationTitle || s.title || '主对话').trim() || '主对话'}
                </span>
                <span className="dialog__tab-token-inline">
                  {' / '}
                  {Number(conversationTokenTotal || 0).toLocaleString()} tokens
                </span>
              </span>
            ) : (
              <span className="dialog__tab-label">{s.title}</span>
            )}
          </button>
        ))}
      </div>

      {agents.length > 0 ? (
        <div className="dialog__agents" aria-label="当前聊天已加入的 agents">
          {agents.map((agent) => (
            <AgentChip key={String(agent?.agent_id ?? agent?.agentID ?? agent?.id ?? '')} agent={agent} />
          ))}
        </div>
      ) : null}
    </div>
  );
}

