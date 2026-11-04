<script lang="ts">
  import type { DistroInfo } from '../types';
  import { IconSweep, IconShield, IconRefresh, IconSettings } from '../icons';
  import { api } from '../api';

  let {
    distro,
    appVersion = '1.0.0',
    safeguardActive = true,
    scanning = false,
    onRefresh = () => {},
    onOpenSettings = () => {},
  }: {
    distro?: DistroInfo | null;
    appVersion?: string;
    safeguardActive?: boolean;
    scanning?: boolean;
    onRefresh?: () => void;
    onOpenSettings?: () => void;
  } = $props();

  const cleanVersion = $derived.by(() => {
    const raw = (appVersion || '').trim();
    if (!raw) return '1.0.0';
    return raw.replace(/^v+/i, '');
  });
</script>

<div
  class="h-11 bg-[#080c13] border-b border-nordic-border flex items-center justify-between px-4 select-none shrink-0"
  style="--wails-draggable: drag;"
>
  <!-- Left Brand & Distro -->
  <div class="flex items-center gap-3">
    <div
      class="w-6 h-6 rounded bg-nordic-cyan-dim border border-nordic-cyan/30 flex items-center justify-center text-nordic-cyan"
    >
      <IconSweep size={14} />
    </div>
    <span class="text-[13px] font-bold text-white tracking-wider">SWEEP</span>
    <span class="text-[11px] font-mono text-nordic-text-muted">v{cleanVersion}</span>
    <span
      class="px-2 py-0.5 rounded text-[11px] font-semibold bg-nordic-purple-dim text-nordic-purple border border-nordic-purple/30"
    >
      {distro?.pretty_name || 'Linux'}
    </span>
  </div>

  <!-- Center Status / Safeguard -->
  <div class="flex items-center gap-2">
    <span
      class="inline-flex items-center gap-1.5 px-2.5 py-0.5 rounded text-[11px] font-semibold {safeguardActive
        ? 'bg-nordic-green-dim text-nordic-green border border-nordic-green/30'
        : 'bg-nordic-amber-dim text-nordic-amber border border-nordic-amber/30'}"
    >
      <IconShield size={12} />
      <span>{safeguardActive ? 'Btrfs Safeguard: Active' : 'Safeguard: Inactive'}</span>
    </span>
  </div>

  <!-- Right Actions & Controls -->
  <div class="flex items-center gap-3" style="--wails-draggable: no-drag;">
    <button
      type="button"
      onclick={onRefresh}
      disabled={scanning}
      title="Rescan system"
      class="p-1.5 text-nordic-text-secondary hover:text-white hover:bg-white/5 rounded transition disabled:opacity-50"
    >
      <IconRefresh size={13} class={scanning ? 'animate-spin text-nordic-cyan' : ''} />
    </button>
    <button
      type="button"
      onclick={onOpenSettings}
      title="Settings & Retention Policies"
      class="p-1.5 text-nordic-text-secondary hover:text-white hover:bg-white/5 rounded transition"
    >
      <IconSettings size={14} />
    </button>
    <div class="flex items-center gap-2 pl-2 border-l border-nordic-border/60">
      <button
        type="button"
        title="Minimize window"
        aria-label="Minimize"
        onclick={() => api.minimizeWindow()}
        class="w-3.5 h-3.5 rounded-full bg-[#f59e0b] border border-[#d97706]/80 hover:brightness-125 active:scale-90 transition-all cursor-pointer focus:outline-none"
      ></button>
      <button
        type="button"
        title="Maximize / Restore window"
        aria-label="Maximize or Restore"
        onclick={() => api.toggleMaximizeWindow()}
        class="w-3.5 h-3.5 rounded-full bg-[#10b981] border border-[#059669]/80 hover:brightness-125 active:scale-90 transition-all cursor-pointer focus:outline-none"
      ></button>
      <button
        type="button"
        title="Close Sweep"
        aria-label="Close"
        onclick={() => api.quitApp()}
        class="w-3.5 h-3.5 rounded-full bg-[#ef4444] border border-[#dc2626]/80 hover:brightness-125 active:scale-90 transition-all cursor-pointer focus:outline-none"
      ></button>
    </div>
  </div>
</div>
