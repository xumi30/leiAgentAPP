import type { MessageClassType, MessageClassifyResult } from '../types/chat';

/**
 * 助手侧「工具调用流水」类文案（与 agent 里 SendAssitantMessageOnce 的格式一致），用于紧凑排版。
 */
export function isAssistantToolRoutineMessage(content: string): boolean {
  const s = String(content ?? '').trim();
  if (!s) return false;
  
  // 与 internal/agent/agent.go 中 SendAssitantMessageOnce 的文案一致
  const toolName = String.raw`[^\s，,;；:：]+`;
  const toolOk = new RegExp(`工具\s*${toolName}\s*执行\s*成功`).test(s);
  const toolFail = new RegExp(`工具\s*${toolName}\s*执行\s*失败`).test(s);
  const toolStartWithArgs = new RegExp(`开始\s*调用\s*工具\s*${toolName}\s*,?\s*参数是`).test(s);
  const toolStart = new RegExp(`开始\s*调用\s*工具\s*${toolName}`).test(s);
  
  return toolOk || toolFail || toolStartWithArgs || toolStart;
}

/**
 * 用户输入分类（纯前端启发式，与后端并行规划可后续对齐）
 */
export function classifyUserMessage(text: string, opts: { isStreaming?: boolean } = {}): MessageClassType {
  const t = String(text ?? '').trim();
  if (!t) return 'supplement';

  const isStreaming = Boolean(opts.isStreaming);

  // 控制类：停生成等（走 StopChat，不新开便签）
  if (
    /^(暂停|停|停止|别写了|别生成|等一下|等等|稍等|cancel|stop|halt|中止)$/i.test(t)
  ) {
    return 'control';
  }

  // 补充类：明确要接着当前上下文说
  if (
    /^(补充|另外|还有|对了|顺便|改一下|加上|接着|继续|同上|续写|接着写)/.test(t)
  ) {
    return 'supplement';
  }

  // 显式新开主题
  if (/^(新任务|换个话题|新开|另一个主题|新开个)/.test(t)) {
    return 'newTopic';
  }

  // 默认为新主题
  return 'newTopic';
}

/**
 * 用户输入分类带标签和置信度
 */
export function classifyUserMessageLabel(
  text: string, 
  opts: { isStreaming?: boolean } = {}
): MessageClassifyResult {
  const type = classifyUserMessage(text, opts);
  let label = '';
  let confidence = 0.9;

  switch (type) {
    case 'control':
      label = '控制指令';
      confidence = 0.95;
      break;
    case 'supplement':
      label = '补充内容';
      confidence = 0.85;
      break;
    case 'newTopic':
      label = '新主题';
      confidence = 0.8;
      break;
  }

  return { type, label, confidence };
}

/**
 * 获取当前消息是否应该显示头像
 */
export function shouldShowMessageAvatar(
  messages: Array<{ role: string; timestamp?: unknown; content?: string; agentID?: string }>,
  index: number
): boolean {
  if (index <= 0) return true;
  
  const cur = messages[index];
  const prev = messages[index - 1];
  if (!cur || !prev) return true;
  
  // 检查当前消息是否包含agentID
  const hasAgentIDInContent = cur.content && String(cur.content).trim().startsWith('{agentID:');
  const hasAgentIDField = cur.agentID && String(cur.agentID).trim() !== '';
  
  // 如果当前消息包含agentID，总是显示头像
  if (hasAgentIDInContent || hasAgentIDField) {
    return true;
  }
  
  const curUser = cur.role === 'user';
  const prevUser = prev.role === 'user';
  if (curUser !== prevUser) return true;
  
  // 时间戳比较逻辑（从Dialog.jsx中提取）
  function messageTimestampMs(ts: unknown): number {
    if (ts == null || ts === '') return NaN;
    if (typeof ts === 'number') return Number.isFinite(ts) ? ts : NaN;
    const n = new Date(ts as string).getTime();
    return Number.isFinite(n) ? n : NaN;
  }
  
  const curMs = messageTimestampMs(cur.timestamp);
  const prevMs = messageTimestampMs(prev.timestamp);
  if (Number.isNaN(curMs) || Number.isNaN(prevMs)) return true;
  
  return curMs - prevMs >= 3 * 60 * 1000; // 3分钟间隔
}