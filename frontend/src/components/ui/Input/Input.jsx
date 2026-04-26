import React from 'react';
// import type { InputProps } from "../../../types/ui';

const BASE_STYLES = 'w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-blue-500 focus:border-blue-500 disabled:bg-gray-100 disabled:cursor-not-allowed transition-colors';

export const Input = ({
  value,
  onChange,
  placeholder,
  disabled = false,
  type = 'text',
  ...props
}) => {
  return (
    <input
      type={type}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      placeholder={placeholder}
      disabled={disabled}
      className={BASE_STYLES}
      {...props}
    />
  );
};

export default Input;