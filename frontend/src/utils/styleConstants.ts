// 样式常量定义 - 将Tailwind类名抽离为常量

/**
 * 布局样式常量
 */
export const LAYOUT_STYLES = {
  flexCenter: 'flex items-center justify-center',
  flexBetween: 'flex items-center justify-between',
  flexStart: 'flex items-center justify-start',
  flexEnd: 'flex items-center justify-end',
  flexCol: 'flex flex-col',
  flexRow: 'flex flex-row',
};

/**
 * 间距常量
 */
export const SPACING_STYLES = {
  p2: 'p-2',
  p4: 'p-4',
  p6: 'p-6',
  px4: 'px-4',
  py2: 'py-2',
  py4: 'py-4',
  my2: 'my-2',
  my4: 'my-4',
  mx2: 'mx-2',
  mx4: 'mx-4',
  gap2: 'gap-2',
  gap4: 'gap-4',
  gap6: 'gap-6',
};

/**
 * 文本样式常量
 */
export const TEXT_STYLES = {
  textXs: 'text-xs',
  textSm: 'text-sm',
  textBase: 'text-base',
  textLg: 'text-lg',
  textXl: 'text-xl',
  fontNormal: 'font-normal',
  fontMedium: 'font-medium',
  fontSemibold: 'font-semibold',
  fontBold: 'font-bold',
};

/**
 * 颜色样式常量
 */
export const COLOR_STYLES = {
  // 背景色
  bgWhite: 'bg-white',
  bgGray100: 'bg-gray-100',
  bgGray200: 'bg-gray-200',
  bgBlue500: 'bg-blue-500',
  bgBlue600: 'bg-blue-600',
  bgRed500: 'bg-red-500',
  bgRed600: 'bg-red-600',
  
  // 文字色
  textWhite: 'text-white',
  textGray500: 'text-gray-500',
  textGray800: 'text-gray-800',
  textBlue500: 'text-blue-500',
  
  // 边框色
  borderGray200: 'border border-gray-200',
  borderGray300: 'border border-gray-300',
  borderBlue500: 'border border-blue-500',
  
  // 悬停色
  hoverBgGray300: 'hover:bg-gray-300',
  hoverBgBlue600: 'hover:bg-blue-600',
  hoverBgRed600: 'hover:bg-red-600',
};

/**
 * 交互状态样式
 */
export const INTERACTION_STYLES = {
  focusRing: 'focus:outline-none focus:ring-2 focus:ring-blue-500 focus:ring-offset-2',
  focusBorder: 'focus:outline-none focus:border-blue-500',
  disabled: 'disabled:opacity-50 disabled:cursor-not-allowed',
  transition: 'transition-colors transition-all',
  transitionColors: 'transition-colors',
};

/**
 * 特定组件样式常量
 */
export const COMPONENT_STYLES = {
  // Dialog组件
  dialogContainer: 'flex flex-col h-full w-full overflow-hidden mx-1.5 rounded-2xl bg-gray-50 border border-gray-200 shadow-sm',
  dialogContent: 'flex-1 overflow-hidden p-4',
  
  // 消息气泡
  messageBubbleBase: 'max-w-[70%] rounded-lg px-4 py-2',
  messageBubbleUser: 'bg-blue-500 text-white ml-12',
  messageBubbleAssistant: 'bg-gray-100 text-gray-900 mr-12 border border-gray-200',
  
  // 头像
  avatarSm: 'w-6 h-6 rounded-full flex-shrink-0',
  avatarMd: 'w-8 h-8 rounded-full flex-shrink-0',
  avatarLg: 'w-12 h-12 rounded-full flex-shrink-0',
  
  // 输入框
  inputBase: 'w-full px-3 py-2 border border-gray-300 rounded-md shadow-sm placeholder-gray-400 focus:outline-none focus:ring-blue-500 focus:border-blue-500',
  textareaBase: 'w-full px-4 py-3 border border-gray-300 rounded-lg resize-none focus:outline-none focus:ring-2 focus:ring-blue-500 focus:border-transparent',
  
  // 按钮
  buttonBase: 'inline-flex items-center justify-center font-medium rounded-md border transition-colors focus:outline-none focus:ring-2 focus:ring-offset-2',
  buttonSm: 'px-3 py-1 text-sm',
  buttonMd: 'px-4 py-2 text-base',
  buttonLg: 'px-6 py-3 text-lg',
};

/**
 * 动画样式常量
 */
export const ANIMATION_STYLES = {
  spin: 'animate-spin',
  pulse: 'animate-pulse',
  bounce: 'animate-bounce',
  ping: 'animate-ping',
};

/**
 * 工具类 - 组合常用样式
 */
export const UTILITY_CLASSES = {
  // 常用按钮组合
  buttonPrimary: `${COMPONENT_STYLES.buttonBase} ${COMPONENT_STYLES.buttonMd} ${COLOR_STYLES.bgBlue500} ${COLOR_STYLES.hoverBgBlue600} ${COLOR_STYLES.textWhite} ${INTERACTION_STYLES.focusRing}`,
  buttonSecondary: `${COMPONENT_STYLES.buttonBase} ${COMPONENT_STYLES.buttonMd} ${COLOR_STYLES.bgGray200} ${COLOR_STYLES.hoverBgGray300} ${COLOR_STYLES.textGray800} ${INTERACTION_STYLES.focusRing}`,
  buttonDanger: `${COMPONENT_STYLES.buttonBase} ${COMPONENT_STYLES.buttonMd} ${COLOR_STYLES.bgRed500} ${COLOR_STYLES.hoverBgRed600} ${COLOR_STYLES.textWhite} ${INTERACTION_STYLES.focusRing}`,
  
  // 输入框组合
  inputStandard: `${COMPONENT_STYLES.inputBase} ${INTERACTION_STYLES.disabled}`,
  textareaStandard: `${COMPONENT_STYLES.textareaBase} ${INTERACTION_STYLES.disabled}`,
  
  // 布局组合
  flexColCenter: `${LAYOUT_STYLES.flexCol} ${LAYOUT_STYLES.flexCenter}`,
  flexRowBetween: `${LAYOUT_STYLES.flexRow} ${LAYOUT_STYLES.flexBetween}`,
};

/**
 * 响应式样式常量
 */
export const RESPONSIVE_STYLES = {
  mobile: {
    messageBubble: 'max-w-[85%]',
    dialogContainer: 'mx-0 rounded-none border-none',
  },
  tablet: {
    messageBubble: 'max-w-[75%]',
  },
  desktop: {
    messageBubble: 'max-w-[70%]',
  },
};