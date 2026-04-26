import React from 'react';

const ThreeColumnLayout = ({ 
  leftSidebar, 
  mainContent, 
  rightSidebar, 
  className = '' 
}) => {
  return (
    <div className={`three-column-layout ${className}`}>
      <div className="layout-left-sidebar">
        {leftSidebar}
      </div>
      <div className="layout-main-content">
        {mainContent}
      </div>
      <div className="layout-right-sidebar">
        {rightSidebar}
      </div>
    </div>
  );
};

export default ThreeColumnLayout;