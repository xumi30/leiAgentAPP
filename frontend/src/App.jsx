import { useState, useEffect } from 'react';

import './App.css';

import ConversationList from './componentjs/ConversationList.jsx';
import Dialog from './componentjs/Dialog.jsx';
import Header from './componentjs/Header.jsx';
import Reasoning from './componentjs/Reasonging.jsx';

function App() {

    const [leftWidth, setLeftWidth] = useState(260); // ConversationList宽度
    const [rightWidth, setRightWidth] = useState(800); // Reasoning宽度
    const [isDragging, setIsDragging] = useState(null); // 当前拖动的边界: 'left' 或 'right'

    const handleMouseDown = (e, border) => {
        e.preventDefault();
        setIsDragging(border);
    };

    useEffect(() => {
        const handleMouseMove = (e) => {
            if (!isDragging) return;

            const container = document.querySelector('.main-content');
            const containerRect = container.getBoundingClientRect();

            if (isDragging === 'left') {
                const newLeftWidth = e.clientX - containerRect.left;
                setLeftWidth(Math.max(100, Math.min(400, newLeftWidth)));
            } else if (isDragging === 'right') {
                const newRightWidth = containerRect.right - e.clientX;
                setRightWidth(Math.max(110, Math.min(1500, newRightWidth)));
            }
        };

        const handleMouseUp = () => {
            setIsDragging(null);
        };

        if (isDragging) {
            document.addEventListener('mousemove', handleMouseMove);
            document.addEventListener('mouseup', handleMouseUp);
        }

        return () => {
            document.removeEventListener('mousemove', handleMouseMove);
            document.removeEventListener('mouseup', handleMouseUp);
        };
    }, [isDragging]);


    return (
        <div id="App" className='board-column'>
            <Header
                isConnected={true}
                showReasoningPanel={true}
            />
            <div className="main-content">
                <div style={{ width: `${leftWidth}px`, minWidth: '200px', maxWidth: '400px' }}>
                    <ConversationList />
                </div>
                <div
                    className="resizer left-resizer"
                    onMouseDown={(e) => handleMouseDown(e, 'left')}
                />
                <div style={{ flex: 1 }}>
                    <Dialog />
                </div>
                <div
                    className="resizer right-resizer"
                    onMouseDown={(e) => handleMouseDown(e, 'right')}
                />
                <div style={{ width: `${rightWidth}px`, minWidth: '100px', maxWidth: '1500px' }}>
                    <Reasoning />
                </div>
            </div>

        </div>
    )
}

export default App
