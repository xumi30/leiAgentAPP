import React from 'react';

export default class AppErrorBoundary extends React.Component {
  constructor(props) {
    super(props);
    this.state = {
      error: null,
      info: null,
    };
  }

  static getDerivedStateFromError(error) {
    return { error };
  }

  componentDidCatch(error, info) {
    console.error('AppErrorBoundary:', error, info);
    this.setState({ info });
  }

  render() {
    const { error, info } = this.state;
    if (error) {
      return (
        <div
          style={{
            minHeight: '100vh',
            boxSizing: 'border-box',
            padding: '24px',
            background: '#fff7f7',
            color: '#6b1f1f',
            fontFamily: 'Nunito, sans-serif',
          }}
        >
          <h1 style={{ margin: '0 0 12px', fontSize: '22px' }}>界面发生异常，但进程还在运行</h1>
          <p style={{ margin: '0 0 12px', lineHeight: 1.6 }}>
            这通常说明前端渲染报错导致页面白屏。请把下面的信息发给我，我继续顺着这个异常点修。
          </p>
          <pre
            style={{
              whiteSpace: 'pre-wrap',
              wordBreak: 'break-word',
              background: '#fff',
              border: '1px solid rgba(107,31,31,0.15)',
              borderRadius: '12px',
              padding: '16px',
              lineHeight: 1.5,
            }}
          >
            {String(error?.stack || error?.message || error)}
            {info?.componentStack ? `\n\nComponent stack:\n${info.componentStack}` : ''}
          </pre>
        </div>
      );
    }

    return this.props.children;
  }
}
