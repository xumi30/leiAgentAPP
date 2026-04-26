import React from 'react';

const Header = ({ children, className = '' }) => {
  return (
    <div className={`header ${className}`}>
      {children}
    </div>
  );
};

export default Header;