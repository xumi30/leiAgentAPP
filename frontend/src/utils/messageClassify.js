/**
 * 助手侧「工具调用流水」类文案（与 agent 里 SendAssitantMessageOnce 的格式一致），用于紧凑排版。
 * @param {string} content
 */
export function isAssistantToolRoutineMessage(content) {
  const s = String(content ?? '').trim();
  if (!s) return false;
  // 与 internal/agent/agent.go 中 SendAssitantMessageOnce 的文案一致
  // 注意：UI 里这些行有时会带前缀/换行/不可见字符，不能用 ^ 强锚定开头；改为“包含式”匹配。
  // 注意：不要用 \b 去卡中文词尾——JS 的 \b 只对 ASCII “单词字符”有效，
  // 「成功」后跟空格/`(` 时 \b 往往匹配不上，会导致整段规则失效。
  const toolName = String.raw`[^\s，,;；:：]+`;
  const toolOk = new RegExp(`工具\\s*${toolName}\\s*执行\\s*成功`).test(s);
  const toolFail = new RegExp(`工具\\s*${toolName}\\s*执行\\s*失败`).test(s);
  const toolStartWithArgs = new RegExp(`开始\\s*调用\\s*工具\\s*${toolName}\\s*,?\\s*参数是`).test(s);
  const toolStart = new RegExp(`开始\\s*调用\\s*工具\\s*${toolName}`).test(s);
  return toolOk || toolFail || toolStartWithArgs || toolStart;
}

/**
 * 用户输入分类（纯前端启发式，与后端并行规划可后续对齐）
 * @param {string} text
 * @param {{ isStreaming?: boolean }} opts
 * @returns {'control' | 'supplement' | 'newTopic'}
 */
export function classifyUserMessage(text, opts = {}) {
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

  // 正在输出时：默认仍视为补充，避免误判导致每条消息都新开便签。
  // 如需新开便签，请使用上面的“显式新开主题”前缀。
  void isStreaming;

  return 'supplement';
}

export function classifyUserMessageLabel(kind) {
  switch (kind) {
    case 'control':
      return '控制';
    case 'newTopic':
      return '新便签';
    case 'supplement':
    default:
      return '补充';
  }
}
