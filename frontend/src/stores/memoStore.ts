import { create } from 'zustand';
import type { MemoState, MemoPreset } from '../types/memo';

interface MemoActions {
  setMemoStripOpen: (open: boolean) => void;
  setMemoCustomPresets: (presets: MemoPreset[]) => void;
  loadCustomMemoPresets: () => void;
  saveCustomMemoPresets: () => void;
  addCustomMemoPreset: (preset: Omit<MemoPreset, 'id'>) => void;
  removeCustomMemoPreset: (id: string) => void;
  setMemoReferencedMessages: (messageIds: string[]) => void;
}

export const useMemoStore = create<MemoState & MemoActions>((set, get) => ({
  // 状态
  memoStripOpen: false,
  memoCustomPresets: [],
  memoReferencedMessages: [],

  // 操作方法
  setMemoStripOpen: (memoStripOpen) => set({ memoStripOpen }),
  
  setMemoCustomPresets: (memoCustomPresets) => set({ memoCustomPresets }),
  
  loadCustomMemoPresets: () => {
    try {
      const raw = localStorage.getItem('leiAgent.memoComposeCustomPresets.v1');
      if (!raw) return;
      
      const arr = JSON.parse(raw);
      if (!Array.isArray(arr)) return;
      
      const presets = arr
        .filter((p) => p && typeof p.label === 'string' && typeof p.text === 'string')
        .map((p) => ({
          id: typeof p.id === 'string' && p.id ? p.id : `u:${Date.now()}_${Math.random().toString(36).slice(2, 9)}`,
          label: p.label.trim().slice(0, 24),
          text: p.text.trim().slice(0, 800),
        }))
        .filter((p) => p.label && p.text);
      
      set({ memoCustomPresets: presets });
    } catch {
      set({ memoCustomPresets: [] });
    }
  },
  
  saveCustomMemoPresets: () => {
    const { memoCustomPresets } = get();
    try {
      localStorage.setItem('leiAgent.memoComposeCustomPresets.v1', JSON.stringify(memoCustomPresets));
    } catch (error) {
      console.error('保存备忘预设失败:', error);
    }
  },
  
  addCustomMemoPreset: (preset) => {
    const newPreset: MemoPreset = {
      id: `u:${Date.now()}_${Math.random().toString(36).slice(2, 9)}`,
      ...preset
    };
    
    set((state) => ({
      memoCustomPresets: [...state.memoCustomPresets, newPreset]
    }));
    
    // 自动保存
    get().saveCustomMemoPresets();
  },
  
  removeCustomMemoPreset: (id) => {
    set((state) => ({
      memoCustomPresets: state.memoCustomPresets.filter(preset => preset.id !== id)
    }));
    
    // 自动保存
    get().saveCustomMemoPresets();
  },
  
  setMemoReferencedMessages: (memoReferencedMessages) => set({ memoReferencedMessages }),
}));