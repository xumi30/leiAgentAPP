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
