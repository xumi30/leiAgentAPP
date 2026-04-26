import { create } from 'zustand';
import type { UIState } from '../types/ui';

interface UIActions {
  setLoading: (loading: boolean) => void;
  setError: (error: string | null) => void;
  setModalOpen: (open: boolean) => void;
  setModalContent: (content?: React.ReactNode) => void;
  openModalWithContent: (content: React.ReactNode) => void;
  closeModal: () => void;
  clearError: () => void;
}

export const useUIStore = create<UIState & UIActions>((set) => ({
  // 状态
  loading: false,
  error: null,
  modalOpen: false,
  modalContent: undefined,

  // 操作方法
  setLoading: (loading) => set({ loading }),
  
  setError: (error) => set({ error }),
  
  setModalOpen: (modalOpen) => set({ modalOpen }),
  
  setModalContent: (modalContent) => set({ modalContent }),
  
  openModalWithContent: (modalContent) => set({ 
    modalOpen: true, 
    modalContent 
  }),
  
  closeModal: () => set({ 
    modalOpen: false, 
    modalContent: undefined 
  }),
  
  clearError: () => set({ error: null }),
}));