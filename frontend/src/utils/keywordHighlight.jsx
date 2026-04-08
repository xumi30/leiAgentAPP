import React, { Children, Fragment, cloneElement, isValidElement } from 'react';
import intentLexicon from './intentLexicon.json';

/**
 * 英文词（带 \\b）；中文词单独列出，按长度降序避免短词抢先。
 * 刻意不收：过短、极常见、易误伤的词（如单独「无法」「可能」「的」）。
 */

/** 用户词表 JSON 键 → msg-kw--* 后缀；同词多类时按 INTENT_CATEGORY_ORDER 先出现者优先 */
const INTENT_CATEGORY_ORDER = [
  '用户指令动作词',
  '意图切换打断纠正词',
  '时间关键词',
  '地点对象资源词',
  '任务计划步骤词',
  '确认疑问需求澄清',
  '否定拒绝限定词',
  '优先级重要性紧急度',
  '情绪态度感受词',
  '专业操作技术类',
  '约束条件范围词',
  '热点话题传播词',
];

const INTENT_KEY_TO_CLASS = {
  用户指令动作词: 'intent',
  意图切换打断纠正词: 'interrupt',
  时间关键词: 'time',
  地点对象资源词: 'resource',
  任务计划步骤词: 'task',
  确认疑问需求澄清: 'clarify',
  否定拒绝限定词: 'negate',
  优先级重要性紧急度: 'priority',
  情绪态度感受词: 'affect',
  专业操作技术类: 'tech',
  约束条件范围词: 'constraint',
  热点话题传播词: 'trending',
};

function buildIntentWordMap() {
  const map = new Map();
  for (const key of INTENT_CATEGORY_ORDER) {
    const cls = INTENT_KEY_TO_CLASS[key];
    const words = intentLexicon[key];
    if (!cls || !words) continue;
    for (const w of words) {
      if (!map.has(w)) map.set(w, cls);
      if (/^[\x00-\x7F]+$/.test(w)) {
        const lower = w.toLowerCase();
        if (!map.has(lower)) map.set(lower, cls);
      }
    }
  }
  return map;
}

const intentWordToClass = buildIntentWordMap();

function intentToneFromMatch(raw) {
  if (intentWordToClass.has(raw)) return intentWordToClass.get(raw);
  const lower = raw.toLowerCase();
  if (/^[\x00-\x7F]+$/.test(raw) && intentWordToClass.has(lower)) {
    return intentWordToClass.get(lower);
  }
  return null;
}

function isAsciiToken(w) {
  return /^[\x00-\x7F]+$/.test(w);
}

const EN_OK = [
  'successfully',
  'success',
  'succeeded',
  'completed',
  'complete',
  'passed',
  'pass',
  'resolved',
  'verified',
  'healthy',
  'enabled',
  'active',
  'applied',
  'installed',
  'restored',
  'fixed',
  'ready',
  'saved',
  'synced',
  'connected',
  'updated',
  'created',
  'deleted',
  'submitted',
  'valid',
  'done',
  'ok',
  'normal',
];

const EN_ERR = [
  'unauthorized',
  'forbidden',
  'unavailable',
  'exception',
  'exceptions',
  'overflow',
  'corrupted',
  'crashed',
  'aborted',
  'denied',
  'timeout',
  'invalid',
  'failure',
  'failures',
  'failed',
  'fatal',
  'errors',
  'error',
  'fail',
  '503',
  '502',
  '500',
  '404',
  '403',
  '401',
];

const EN_WARN = [
  'deprecated',
  'cancelled',
  'canceled',
  'warnings',
  'warning',
  'caution',
  'retry',
  'skipped',
  'stopped',
  'limited',
  'slow',
  'warn',
];

const EN_INFO = [
  'processing',
  'uploading',
  'downloading',
  'syncing',
  'waiting',
  'running',
  'loading',
  'pending',
  'queued',
  'optional',
  'default',
  'example',
  'details',
  'hints',
  'hint',
  'tips',
  'tip',
  'info',
  'note',
];

const CN_OK = [
  '已成功',
  '已完成',
  '已保存',
  '已同步',
  '已连接',
  '已更新',
  '已创建',
  '已删除',
  '已提交',
  '已生效',
  '已应用',
  '已安装',
  '已就绪',
  '已恢复',
  '已修复',
  '已解决',
  '无错误',
  '无异常',
  '成功',
  '完成',
  '通过',
  '正常',
  '匹配',
  '有效',
  '正确',
  '确认',
  '启用',
  '解决',
  '顺利',
  '安全',
  '健康',
  '就绪',
];

const CN_ERR = [
  '拒绝访问',
  '校验失败',
  '未授权',
  '不可用',
  '未找到',
  '超时',
  '非法',
  '冲突',
  '过期',
  '拒绝',
  '无效',
  '缺失',
  '崩溃',
  '中断',
  '断连',
  '断开',
  '失败',
  '错误',
  '异常',
];

const CN_WARN = [
  '兼容性',
  '请确认',
  '需确认',
  '已取消',
  '已停止',
  '已跳过',
  '待确认',
  '警告',
  '注意',
  '谨慎',
  '建议',
  '待定',
  '跳过',
  '重试',
  '降级',
  '兼容',
  '风险',
  '提醒',
  '过时',
  '限制',
  '慢',
];

const CN_INFO = [
  '请稍候',
  '请稍等',
  '加载中',
  '处理中',
  '运行中',
  '同步中',
  '上传中',
  '下载中',
  '排队中',
  '进行中',
  '提示',
  '详见',
  '示例',
];

/** 流程起止、启停（绿色系，与「成功」略区分） */
const EN_FLOW = [
  'terminated',
  'terminating',
  'terminate',
  'launching',
  'launched',
  'launch',
  'resumed',
  'resume',
  'paused',
  'pausing',
  'pause',
  'starting',
  'started',
  'beginning',
  'began',
  'begin',
  'start',
  'finishing',
  'finished',
  'finish',
  'ending',
  'ended',
  'end',
  'halt',
  'stop',
];

const CN_FLOW = [
  '重新开始',
  '已经结束',
  '已开始',
  '已结束',
  '已启动',
  '开始',
  '启动',
  '开端',
  '结束',
  '终止',
  '暂停',
  '继续',
  '收尾',
  '完结',
  '开场',
  '闭幕',
  '停止',
];

/** 快乐 / 愉悦 / 活力：明亮黄、暖橙、浅粉 */
const EN_EMO_JOY = [
  'wonderful',
  'awesome',
  'amazing',
  'cheerful',
  'delighted',
  'delight',
  'energetic',
  'exciting',
  'excited',
  'vibrant',
  'lively',
  'happiness',
  'joyful',
  'joy',
  'happy',
  'pleased',
  'satisfied',
  'excellent',
  'great',
  'glad',
  'fun',
];

const CN_EMO_JOY = [
  '太棒了',
  '太好了',
  '真好',
  '快乐',
  '愉悦',
  '活力',
  '兴奋',
  '开心',
  '高兴',
  '满意',
  '惊喜',
  '赞',
  '棒',
];

/** 幸福 / 温馨 / 治愈：奶油、浅杏、柔粉、暖米 */
const EN_EMO_WARM = [
  'heartwarming',
  'wholesome',
  'blessed',
  'cozy',
  'grateful',
  'appreciate',
  'appreciated',
  'thanks',
  'thank',
];

const CN_EMO_WARM = [
  '幸福',
  '温馨',
  '感动',
  '温暖',
  '安心',
  '治愈',
  '感谢',
  '谢谢',
  '欣慰',
];

/** 希望 / 积极 / 向上：嫩绿、天青、浅蓝 */
const EN_EMO_HOPE = [
  'optimistic',
  'positive',
  'hopeful',
  'hope',
  'brighter',
  'forward',
];

const CN_EMO_HOPE = [
  '积极向上',
  '希望',
  '期待',
  '积极',
  '向上',
  '加油',
  '成长',
];

/** 自信 / 力量 / 坚定：正红、藏蓝、墨绿、深橙 + 金辅 */
const EN_EMO_POWER = [
  'determination',
  'determined',
  'confidence',
  'confident',
  'powerful',
  'power',
  'strength',
  'strong',
  'courage',
  'courageous',
  'brave',
  'bold',
];

const CN_EMO_POWER = [
  '自信',
  '力量',
  '坚定',
  '勇敢',
  '强大',
  '必胜',
];

/** 浪漫 / 心动 / 温柔：玫瑰粉、淡紫、藕粉 */
const EN_EMO_ROMANTIC = [
  'romantic',
  'romance',
  'crush',
  'loved',
  'love',
];

const CN_EMO_ROMANTIC = [
  '浪漫',
  '心动',
  '喜欢',
  '爱你',
  '甜蜜',
  '温柔',
  '柔情',
];

/** 平静 / 放松 / 治愈：雾霾蓝、薄荷绿、浅青 */
const EN_EMO_CALM = [
  'peaceful',
  'serenity',
  'serene',
  'soothing',
  'relaxed',
  'relax',
  'calmness',
  'calm',
];

const CN_EMO_CALM = [
  '平静',
  '放松',
  '舒缓',
  '宁静',
];

/** 消极情感 */
const EN_EMO_NEG = [
  'frustrated',
  'frustration',
  'disappointed',
  'disappointment',
  'anxious',
  'anxiety',
  'terrible',
  'awful',
  'hateful',
  'hate',
  'afraid',
  'fear',
  'worried',
  'worry',
  'anger',
  'angry',
  'sadness',
  'sad',
  'sorry',
];

const CN_EMO_NEG = [
  '失望',
  '焦虑',
  '害怕',
  '生气',
  '难过',
  '郁闷',
  '痛苦',
  '悲伤',
  '遗憾',
  '讨厌',
  '担心',
  '烦',
  '恨',
  '可恶',
];

function escapeRe(s) {
  return s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
}

function buildKeywordRegex() {
  const en = [
    ...EN_OK,
    ...EN_ERR,
    ...EN_WARN,
    ...EN_INFO,
    ...EN_FLOW,
    ...EN_EMO_JOY,
    ...EN_EMO_WARM,
    ...EN_EMO_HOPE,
    ...EN_EMO_POWER,
    ...EN_EMO_ROMANTIC,
    ...EN_EMO_CALM,
    ...EN_EMO_NEG,
  ];
  const cn = [
    ...CN_OK,
    ...CN_ERR,
    ...CN_WARN,
    ...CN_INFO,
    ...CN_FLOW,
    ...CN_EMO_JOY,
    ...CN_EMO_WARM,
    ...CN_EMO_HOPE,
    ...CN_EMO_POWER,
    ...CN_EMO_ROMANTIC,
    ...CN_EMO_CALM,
    ...CN_EMO_NEG,
  ];
  for (const key of INTENT_CATEGORY_ORDER) {
    const words = intentLexicon[key];
    if (!words) continue;
    for (const w of words) {
      if (isAsciiToken(w)) en.push(w);
      else cn.push(w);
    }
  }
  const enSorted = [...new Set(en)].sort((a, b) => b.length - a.length);
  const cnSorted = [...new Set(cn)].sort((a, b) => b.length - a.length);
  const enPart = enSorted.map((w) => `\\b${escapeRe(w)}\\b`).join('|');
  const cnPart = cnSorted.map(escapeRe).join('|');
  return new RegExp(`(${enPart}|${cnPart})`, 'gi');
}

let cachedRe = null;
function getKeywordRe() {
  if (!cachedRe) cachedRe = buildKeywordRegex();
  return cachedRe;
}

/** @param {string} m 本次匹配到的原文 */
function keywordToneClass(m) {
  const raw = String(m);
  const lower = raw.toLowerCase();
  if (EN_ERR.some((w) => lower === w) || CN_ERR.includes(raw)) return 'err';
  if (EN_OK.some((w) => lower === w) || CN_OK.includes(raw)) return 'ok';
  if (EN_WARN.some((w) => lower === w) || CN_WARN.includes(raw)) return 'warn';
  if (EN_FLOW.some((w) => lower === w) || CN_FLOW.includes(raw)) return 'flow';
  if (EN_EMO_JOY.some((w) => lower === w) || CN_EMO_JOY.includes(raw)) return 'emo-joy';
  if (EN_EMO_WARM.some((w) => lower === w) || CN_EMO_WARM.includes(raw)) return 'emo-warm';
  if (EN_EMO_HOPE.some((w) => lower === w) || CN_EMO_HOPE.includes(raw)) return 'emo-hope';
  if (EN_EMO_POWER.some((w) => lower === w) || CN_EMO_POWER.includes(raw)) return 'emo-power';
  if (EN_EMO_ROMANTIC.some((w) => lower === w) || CN_EMO_ROMANTIC.includes(raw)) return 'emo-romantic';
  if (EN_EMO_CALM.some((w) => lower === w) || CN_EMO_CALM.includes(raw)) return 'emo-calm';
  if (EN_EMO_NEG.some((w) => lower === w) || CN_EMO_NEG.includes(raw)) return 'emo-neg';
  if (EN_INFO.some((w) => lower === w) || CN_INFO.includes(raw)) return 'info';
  const intentCls = intentToneFromMatch(raw);
  if (intentCls) return intentCls;
  return 'info';
}

/**
 * 将纯文本中的状态词拆成片段并包一层 span
 * @param {string} text
 */
export function splitTextWithKeywordHighlights(text) {
  if (text == null || text === '') return text;
  const re = getKeywordRe();
  const parts = String(text).split(re);
  if (parts.length === 1) return text;

  return parts.map((part, i) => {
    if (i % 2 === 0) return part;
    const tone = keywordToneClass(part);
    return (
      <span key={`kw-${i}`} className={`msg-kw msg-kw--${tone}`}>
        {part}
      </span>
    );
  });
}

/**
 * 递归处理 React 子节点：仅对字符串做高亮；跳过 code / pre 内文字
 * @param {React.ReactNode} children
 */
export function highlightKeywordChildren(children) {
  return Children.map(children, (child, i) => {
    if (child == null || child === false) return child;
    if (typeof child === 'string') {
      const h = splitTextWithKeywordHighlights(child);
      if (h === child) return child;
      return <Fragment key={`hk-${i}`}>{h}</Fragment>;
    }
    if (typeof child === 'number') return child;
    if (!isValidElement(child)) return child;

    const tag = child.type;
    if (tag === 'code' || tag === 'pre') {
      return cloneElement(child, { key: child.key ?? `nc-${i}` });
    }
    if (child.props?.children != null) {
      return cloneElement(child, { key: child.key ?? `hk-${i}` }, highlightKeywordChildren(child.props.children));
    }
    return child;
  });
}
