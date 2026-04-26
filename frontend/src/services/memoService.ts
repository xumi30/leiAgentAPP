import { apiCall, AppendMemoMarkdown, ComposeMemoWithLLM, GetMemoReferencedMessageIDs } from './api';

export class MemoService {
  
  /**
   * 将Markdown内容追加到备忘录
   */
  static async appendMemoMarkdown(content: string): Promise<any> {
    return apiCall(
      () => AppendMemoMarkdown(content),
      '追加备忘录内容失败'
    );
  }

  /**
   * 使用LLM生成备忘录内容
   */
  static async composeMemoWithLLM(
    prompt: string,
    referencedMessageIds?: string[]
  ): Promise<any> {
    return apiCall(
      () => ComposeMemoWithLLM(prompt, referencedMessageIds),
      'LLM生成备忘录失败'
    );
  }

  /**
   * 获取备忘录引用的消息ID
   */
  static async getMemoReferencedMessageIDs(): Promise<any> {
    return apiCall(
      () => GetMemoReferencedMessageIDs(),
      '获取备忘录引用消息失败'
    );
  }

  /**
   * 加载自定义预设
   */
  static loadCustomPresets(): Array<{ id: string; label: string; text: string }> {
    try {
      const raw = localStorage.getItem('leiAgent.memoComposeCustomPresets.v1');
      if (!raw) return [];
      
      const arr = JSON.parse(raw);
      if (!Array.isArray(arr)) return [];
      
      return arr
        .filter((p) => p && typeof p.label === 'string' && typeof p.text === 'string')
        .map((p) => ({
          id: typeof p.id === 'string' && p.id ? p.id : `u:${Date.now()}_${Math.random().toString(36).slice(2, 9)}`,
          label: p.label.trim().slice(0, 24),
          text: p.text.trim().slice(0, 800),
        }))
        .filter((p) => p.label && p.text);
    } catch {
      return [];
    }
  }

  /**
   * 保存自定义预设
   */
  static saveCustomPresets(presets: Array<{ id: string; label: string; text: string }>): void {
    try {
      localStorage.setItem('leiAgent.memoComposeCustomPresets.v1', JSON.stringify(presets));
    } catch (error) {
      console.error('保存备忘录预设失败:', error);
    }
  }

  /**
   * 预设验证
   */
  static validatePreset(preset: { label: string; text: string }): boolean {
    return (
      preset.label.trim().length > 0 &&
      preset.label.trim().length <= 24 &&
      preset.text.trim().length > 0 &&
      preset.text.trim().length <= 800
    );
  }
}