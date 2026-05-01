const MACARON_COLORS = [
  { bg: '#F2D4D9', text: '#705559' },
  { bg: '#CFE6E2', text: '#4E6662' },
  { bg: '#D1DFF2', text: '#556478' },
  { bg: '#DFD7F2', text: '#5E5673' },
  { bg: '#E6EAD8', text: '#575E48' },
  { bg: '#F5E4D7', text: '#725E4F' },
  { bg: '#EBD6E6', text: '#6A5464' },
  { bg: '#F1D8D2', text: '#715750' },
  { bg: '#D6E7F0', text: '#50606B' },
  { bg: '#E4E0EF', text: '#58536B' },
  { bg: '#F8D3D7', text: '#6D4C4F' },
  { bg: '#D1E9DD', text: '#4A6157' },
  { bg: '#D4E5F5', text: '#52647A' },
  { bg: '#E2D8F0', text: '#5C5270' },
  { bg: '#F5E6CF', text: '#705C45' },
  { bg: '#F0D8E8', text: '#6B4E5F' },
  { bg: '#D1CCE8', text: '#4A4763' },
  { bg: '#E4E9D7', text: '#555C44' },
  { bg: '#FFE5E1', text: '#70504A' },
  { bg: '#D8E8E9', text: '#475A5B' },
  { bg: 'rgba(248,211,215,0.85)', text: '#6D4C4F' },
  { bg: 'rgba(209,233,221,0.85)', text: '#4A6157' },
  { bg: 'rgba(212,229,245,0.85)', text: '#52647A' },
  { bg: 'rgba(226,216,240,0.85)', text: '#5C5270' },
  { bg: 'rgba(242,212,217,0.8)', text: '#705559' },
  { bg: 'rgba(207,230,226,0.8)', text: '#4E6662' },
  { bg: 'rgba(209,223,242,0.8)', text: '#556478' },
  { bg: 'rgba(223,215,242,0.8)', text: '#5E5673' },
  { bg: 'rgba(230,234,216,0.8)', text: '#575E48' },
];

export function getMacaronColor(id) {
  const source = String(id ?? '');
  let hash = 0;
  for (let index = 0; index < source.length; index += 1) {
    hash = source.charCodeAt(index) + ((hash << 5) - hash);
  }
  return MACARON_COLORS[Math.abs(hash) % MACARON_COLORS.length];
}
