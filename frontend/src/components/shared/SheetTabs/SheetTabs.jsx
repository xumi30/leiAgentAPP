import React from 'react';
import { useChatStore } from '../../../stores';
// // import type { Sheet } from "../../../types/chat';

const SheetTabs = ({
  sheets,
  activeSheetId,
  onSheetChange,
  onSheetAdd,
  onSheetRemove
}) => {
  const { MAIN_SHEET_ID } = require('../../../utils/constants');

  return (
    <div className="flex items-center gap-1 overflow-x-auto scrollbar-hide">
      {sheets.map((sheet) => (
        <div
          key={sheet.id}
          className={`
            flex items-center gap-2 px-3 py-2 rounded-lg cursor-pointer transition-all
            ${activeSheetId === sheet.id 
              ? 'bg-blue-500 text-white' 
              : 'bg-gray-100 text-gray-700 hover:bg-gray-200'
            }
          `}
          onClick={() => onSheetChange(sheet.id)}
        >
          <span className="text-sm font-medium whitespace-nowrap">
            {sheet.title}
          </span>
          
          {sheet.id !== MAIN_SHEET_ID && (
            <button
              onClick={(e) => {
                e.stopPropagation();
                onSheetRemove?.(sheet.id);
              }}
              className={`
                w-4 h-4 rounded-full flex items-center justify-center text-xs
                ${activeSheetId === sheet.id 
                  ? 'bg-white text-blue-500' 
                  : 'bg-gray-300 text-gray-600'
                }
              `}
            >
              ×
            </button>
          )}
        </div>
      ))}
      
      {onSheetAdd && (
        <button
          onClick={onSheetAdd}
          className="
            w-8 h-8 rounded-lg flex items-center justify-center 
            bg-gray-100 text-gray-500 hover:bg-gray-200 
            transition-colors
          "
          title="新建标签页"
        >
          +
        </button>
      )}
    </div>
  );
};

export default SheetTabs;