// frontend/src/features/chat/components/Tooltip/Tooltip.jsx
import React, { useState, useRef, useEffect } from 'react';
import './Tooltip.css';

const Tooltip = ({ children, content, position = 'top' }) => {
  const [isVisible, setIsVisible] = useState(false);
  const tooltipRef = useRef(null);
  const containerRef = useRef(null);

  const showTooltip = () => {
    setIsVisible(true);
  };

  const hideTooltip = () => {
    setIsVisible(false);
  };

  // 计算tooltip位置
  const updatePosition = () => {
    if (!containerRef.current || !tooltipRef.current) return;
    
    const containerRect = containerRef.current.getBoundingClientRect();
    const tooltipRect = tooltipRef.current.getBoundingClientRect();
    
    let top, left;
    
    switch (position) {
      case 'top':
        top = containerRect.top - tooltipRect.height - 8;
        left = containerRect.left + (containerRect.width - tooltipRect.width) / 2;
        break;
      case 'bottom':
        top = containerRect.bottom + 8;
        left = containerRect.left + (containerRect.width - tooltipRect.width) / 2;
        break;
      case 'left':
        top = containerRect.top + (containerRect.height - tooltipRect.height) / 2;
        left = containerRect.left - tooltipRect.width - 8;
        break;
      case 'right':
        top = containerRect.top + (containerRect.height - tooltipRect.height) / 2;
        left = containerRect.right + 8;
        break;
      default:
        top = containerRect.top - tooltipRect.height - 8;
        left = containerRect.left + (containerRect.width - tooltipRect.width) / 2;
    }
    
    tooltipRef.current.style.top = `${top}+400px`;
    tooltipRef.current.style.left = `${left}-200px`;
  };

  useEffect(() => {
    if (isVisible) {
      // 确保DOM更新后再计算位置
      requestAnimationFrame(() => {
        updatePosition();
      });
      
      // 添加事件监听器，在窗口大小改变时更新位置
      window.addEventListener('resize', updatePosition);
      window.addEventListener('scroll', updatePosition);
      
      return () => {
        window.removeEventListener('resize', updatePosition);
        window.removeEventListener('scroll', updatePosition);
      };
    }
  }, [isVisible, position]);

  return (
    <div 
      className="tooltip-container" 
      ref={containerRef}
      onMouseEnter={showTooltip}
      onMouseLeave={hideTooltip}
    >
      {children}
      {isVisible && (
        <div className={`tooltip tooltip-${position} visible`} ref={tooltipRef}>
          {content}
        </div>
      )}
    </div>
  );
};

export default Tooltip;
