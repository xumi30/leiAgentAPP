import React from 'react';

// interface removed

const AVATAR_SIZES = {
  sm: 'w-6 h-6',
  md: 'w-8 h-8',
  lg: 'w-12 h-12'
};

export const Avatar = ({
  src,
  alt,
  size = 'md',
  className = ''
}) => {
  const baseStyles = `${AVATAR_SIZES[size]} rounded-full flex-shrink-0`;
  
  return (
    <div className={`${baseStyles} ${className}`}>
      {src ? (
        <img src={src} alt={alt} className="w-full h-full rounded-full object-cover" />
      ) : (
        <div className="w-full h-full rounded-full bg-gray-300 flex items-center justify-center text-gray-600 text-sm font-medium">
          {alt.charAt(0).toUpperCase()}
        </div>
      )}
    </div>
  );
};

export default Avatar;