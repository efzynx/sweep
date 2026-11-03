export function formatBytes(bytes: number, decimals: number = 2): string {
  if (!bytes || bytes <= 0 || isNaN(bytes)) return '0 B';

  const k = 1024;
  const dm = decimals < 0 ? 0 : decimals;
  const sizes = ['B', 'KB', 'MB', 'GB', 'TB', 'PB'];

  const i = Math.floor(Math.log(bytes) / Math.log(k));
  if (i < 0) return '0 B';
  if (i >= sizes.length) return (bytes / Math.pow(k, sizes.length - 1)).toFixed(dm) + ' ' + sizes[sizes.length - 1];

  return parseFloat((bytes / Math.pow(k, i)).toFixed(dm)) + ' ' + sizes[i];
}

export function formatPercent(used: number, total: number): string {
  if (!total || total <= 0) return '0.0%';
  const pct = (used / total) * 100;
  return `${pct.toFixed(1)}%`;
}

export function formatDurationMs(nanoseconds: number): string {
  if (!nanoseconds || nanoseconds <= 0) return '0ms';
  const ms = Math.round(nanoseconds / 1_000_000);
  if (ms < 1000) {
    return `${ms}ms`;
  }
  return `${(ms / 1000).toFixed(2)}s`;
}
