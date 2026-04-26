import React from 'react';
import assistantAvatar from '../../../assets/images/aitx.png';

/**
 * AgentChip
 * @param {{
 *  agent: any,
 * }} props
 */
export default function AgentChip({ agent }) {
  if (!agent) return null;
  const id = String(agent?.agent_id ?? agent?.agentID ?? '').trim();
  const name = String(agent?.agent_name ?? agent?.name ?? id).trim();
  const avatar = String(agent?.avatar_image ?? agent?.avatar ?? '').trim();
  const desc = String(agent?.description ?? '').trim();

  return (
    <div key={id || name} className="dialog__agent-chip" title={desc}>
      <img
        className="dialog__agent-chip-avatar"
        src={avatar || assistantAvatar}
        onError={(e) => {
          if (e.currentTarget?.src !== assistantAvatar) e.currentTarget.src = assistantAvatar;
        }}
        alt={name || 'agent'}
      />
      <span className="dialog__agent-chip-label">{name}</span>
    </div>
  );
}

