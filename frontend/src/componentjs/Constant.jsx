export const colorSchemes = [
  { bg: '#E3F2FD', text: '#1565C0' },
  { bg: '#F3E5F5', text: '#7B1FA2' },
  { bg: '#E8F5E9', text: '#2E7D32' },
  { bg: '#FFF3E0', text: '#EF6C00' },
  { bg: '#FFEBEE', text: '#C62828' },
  { bg: '#E0F7FA', text: '#00838F' },
  { bg: '#F1F8E9', text: '#558B2F' },
  { bg: '#FCE4EC', text: '#AD1457' },
];

export const getRandomColorScheme = () => {
  return colorSchemes[Math.floor(Math.random() * colorSchemes.length)];
};

export const getRandomMacaronClass = () => {
  const classes = [
    'macaron-1',
    'macaron-2',
    'macaron-3',
    'macaron-4',
    'macaron-5',
    'macaron-6'
  ];
  return classes[Math.floor(Math.random() * classes.length)];
};

export const renderMessageContent = (content) => {
  return content.split('\n').map((line, index) => {
    if (line.startsWith('```')) {
      return <pre key={index}><code>{line.substring(3)}</code></pre>;
    } else if (line.startsWith('- ')) {
      return <li key={index}>{line.substring(2)}</li>;
    } else if (line.startsWith('# ')) {
      return <h3 key={index}>{line.substring(2)}</h3>;
    } else {
      return <p key={index}>{line}</p>;
    }
  });
};

const macaronColors = [
  { bg: '#F2D4D9', text: '#705559' }, // 雾面樱花粉
  { bg: '#CFE6E2', text: '#4E6662' }, // 冰感薄荷绿
  { bg: '#D1DFF2', text: '#556478' }, // 雾感天空蓝
  { bg: '#DFD7F2', text: '#5E5673' }, // 灰调薰衣草紫
  { bg: '#E6EAD8', text: '#575E48' }, // 浅豆青绿
  { bg: '#F5E4D7', text: '#725E4F' }, // 奶雾杏色
  { bg: '#EBD6E6', text: '#6A5464' }, // 淡藕荷紫
  { bg: '#F1D8D2', text: '#715750' }, // 柔雾珊瑚色
  { bg: '#D6E7F0', text: '#50606B' }, // 浅冰川蓝
  { bg: '#E4E0EF', text: '#58536B' }, // 浅雾紫

  // 核心四色渐变（浅粉→浅绿→浅蓝→浅紫）
  { bg: '#F8D3D7', text: '#6D4C4F' }, // 柔雾粉（低饱和升级）
  { bg: '#D1E9DD', text: '#4A6157' }, // 薄荷奶绿（高透感）
  { bg: '#D4E5F5', text: '#52647A' }, // 雾感浅蓝（新增，玻璃风首选）
  { bg: '#E2D8F0', text: '#5C5270' }, // 烟紫（莫兰迪低饱和）

  // 治愈系拓展色
  { bg: '#F5E6CF', text: '#705C45' }, // 奶杏色（温柔不刺眼）
  { bg: '#F0D8E8', text: '#6B4E5F' }, // 淡蔷薇
  { bg: '#D1CCE8', text: '#4A4763' }, // 薰衣草雾紫
  { bg: '#E4E9D7', text: '#555C44' }, // 浅豆绿
  { bg: '#FFE5E1', text: '#70504A' }, // 浅珊瑚（柔光版）
  { bg: '#D8E8E9', text: '#475A5B' }, // 冰薄荷蓝（通透感拉满）

  { bg: 'rgba(248,211,215,0.85)', text: '#6D4C4F' }, // 半透柔粉
  { bg: 'rgba(209,233,221,0.85)', text: '#4A6157' }, // 半透薄荷绿
  { bg: 'rgba(212,229,245,0.85)', text: '#52647A' }, // 半透雾蓝
  { bg: 'rgba(226,216,240,0.85)', text: '#5C5270' }, // 半透烟紫

  { bg: 'rgba(242,212,217,0.8)', text: '#705559' },
  { bg: 'rgba(207,230,226,0.8)', text: '#4E6662' },
  { bg: 'rgba(209,223,242,0.8)', text: '#556478' },
  { bg: 'rgba(223,215,242,0.8)', text: '#5E5673' },
  { bg: 'rgba(230,234,216,0.8)', text: '#575E48' },
];

// 随机选择马卡龙配色
export const getRandomMacaronColor = (id) => {
  // 使用简单的哈希算法将 id 转换为数字
  let hash = 0;
  for (let i = 0; i < id.length; i++) {
    hash = id.charCodeAt(i) + ((hash << 5) - hash);
  }
  
  // 确保索引在有效范围内
  const index = Math.abs(hash) % macaronColors.length;
  return macaronColors[index];
};


export const makeChatId = () => {
  return Date.now().toString() + Math.floor(Math.random() * 1000).toString().padStart(3, '0');
}

export const makeMessageId = () => {
  return Date.now().toString() + Math.floor(Math.random() * 10000).toString().padStart(4, '0');
}