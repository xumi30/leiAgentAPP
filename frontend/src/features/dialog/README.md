## Dialog feature module

该目录是从 `frontend/src/componentjs/Dialog.jsx` 拆分出来的“工业化模块”版本：

- `components/`: 纯 UI 子组件（只收 props，不直接调用后端 API）
- `hooks/`: 业务逻辑与副作用（Wails 事件桥接层优先收敛到 `useStore.initWailsEventBridge`）
- `utils/`: 纯函数工具（便于单元测试）
- `constants.js`: 模块级常量
- `index.jsx`: Dialog 主入口（组合 hooks + components）

