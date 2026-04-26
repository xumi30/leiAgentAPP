import React from 'react';
// import type { ButtonProps } from "../../../types/ui';

// 样式常量 - 将Tailwind类名抽离为常量
const BUTTON_VARIANTS = {
  primary: 'bg-blue-500 hover:bg-blue-600 text-white border-blue-600',
  secondary: 'bg-gray-200 hover:bg-gray-300 text-gray-800 border-gray-300',
  danger: 'bg-red-500 hover:bg-red-600 text-white border-red-600'
};

const BUTTON_SIZES = {
  sm: 'px-3 py-1 text-sm',
  md: 'px-4 py-2 text-base',
  lg: 'px-6 py-3 text-lg'
};

const BASE_STYLES = 'inline-flex items-center justify-center font-medium rounded-md border transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2 disabled:opacity-50 disabled:cursor-not-allowed';

export const Button = ({
  variant = 'primary',
  size = 'md',
  loading = false,
  disabled = false,
  onClick,
  children,
  ...props
}) => {
  const className = `${BASE_STYLES} ${BUTTON_VARIANTS[variant]} ${BUTTON_SIZES[size]} ${loading ? 'opacity-50 cursor-not-allowed' : ''}`;
  
  return (
    <button
      className={className}
      onClick={onClick}
      disabled={disabled || loading}
      {...props}
    >
      {loading ? (
        <>
          <svg className="animate-spin -ml-1 mr-2 h-4 w-4" fill="none" viewBox="0 0 24 24">
            <circle className="opacity-25" cx="12" cy="12" r="10" stroke="currentColor" strokeWidth="4" />
            <path className="opacity-75" fill="currentColor" d="M4 12a8 8 0 018-8V0C5.373 0 0 5.373 0 12h4zm2 5.291A7.962 7.962 0 014 12H0c0 3.042 1.135 5.824 3 7.938l3-2.647z" />
          </svg>
          {children}
        </>
      ) : (
        children
      )}
    </button>
  );
};

export default Button;