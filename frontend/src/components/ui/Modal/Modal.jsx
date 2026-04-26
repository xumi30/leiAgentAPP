import React from 'react';

// interface removed

const MODAL_SIZES = {
  sm: 'max-w-md',
  md: 'max-w-lg',
  lg: 'max-w-2xl'
};

const BASE_STYLES = 'fixed inset-0 z-50 overflow-y-auto';
const OVERLAY_STYLES = 'fixed inset-0 bg-black bg-opacity-50 transition-opacity';
const MODAL_CONTAINER_STYLES = 'flex min-h-full items-center justify-center p-4 text-center sm:p-0';
const MODAL_PANEL_STYLES = 'relative transform overflow-hidden rounded-lg bg-white text-left shadow-xl transition-all sm:my-8 w-full';

export const Modal = ({
  isOpen,
  onClose,
  title,
  children,
  size = 'md'
}) => {
  if (!isOpen) return null;

  return (
    <div className={BASE_STYLES}>
      <div className={OVERLAY_STYLES} onClick={onClose} />
      
      <div className={MODAL_CONTAINER_STYLES}>
        <div className={`${MODAL_PANEL_STYLES} ${MODAL_SIZES[size]}`}>
          {title && (
            <div className="bg-gray-50 px-6 py-4 border-b border-gray-200">
              <h3 className="text-lg font-medium text-gray-900">{title}</h3>
            </div>
          )}
          
          <div className="px-6 py-4">
            {children}
          </div>
        </div>
      </div>
    </div>
  );
};

export default Modal;