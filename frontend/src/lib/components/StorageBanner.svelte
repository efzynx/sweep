<script lang="ts">
  import type { SystemStorageInfo, StoragePartitionFilter } from '../types';
  import { formatBytes } from '../format';
  import { IconDisk, IconHome } from '../icons';

  let {
    storage,
    currentFilter = 'all',
    onFilterChange = () => {},
  }: {
    storage?: SystemStorageInfo | null;
    currentFilter?: StoragePartitionFilter;
    onFilterChange?: (filter: StoragePartitionFilter) => void;
  } = $props();

  const rootStats = $derived.by(() => {
    if (!storage?.root) {
      return { free: '0 B', total: '0 B', usedPct: '0.0', pctVal: 0 };
    }
    const r = storage.root;
    const free = formatBytes(r.free_bytes);
    const total = formatBytes(r.total_bytes);
    const pct = r.used_percent ? r.used_percent.toFixed(1) : '0.0';
    return { free, total, usedPct: pct, pctVal: r.used_percent || 0 };
  });

  const homeStats = $derived.by(() => {
    if (!storage?.home) {
      return { free: '0 B', total: '0 B', usedPct: '0.0', pctVal: 0 };
    }
    const h = storage.home;
    const free = formatBytes(h.free_bytes);
    const total = formatBytes(h.total_bytes);
    const pct = h.used_percent ? h.used_percent.toFixed(1) : '0.0';
    return { free, total, usedPct: pct, pctVal: h.used_percent || 0 };
  });

  const hasSeparateHome = $derived(!!storage?.has_separate_home && !!storage?.home?.path);
</script>

<div
  class="bg-[#090e17] border-b border-nordic-border px-5 py-3 grid grid-cols-1 md:grid-cols-[1fr_1fr_auto] gap-5 items-center select-none shrink-0"
>
  <!-- Root Storage Meter -->
  <div class="flex flex-col gap-1.5">
    <div class="flex items-center justify-between text-xs">
      <span class="font-bold text-slate-200 flex items-center gap-1.5">
        <span class="text-nordic-cyan"><IconDisk size={14} /></span>
        <span>Root Partition (/)</span>
      </span>
      <span class="font-mono text-[11.5px] text-nordic-text-secondary">
        Root: {rootStats.free} Free / {rootStats.total} ({rootStats.usedPct}% Used)
      </span>
    </div>
    <div class="h-[7px] bg-[#162030] rounded overflow-hidden">
      <div
        class="h-full rounded bg-gradient-to-r from-[#06b6d4] to-[#0891b2] transition-all duration-500 ease-out"
        style="width: {rootStats.pctVal}%;"
      ></div>
    </div>
  </div>

  <!-- Home Storage Meter -->
  {#if hasSeparateHome}
    <div class="flex flex-col gap-1.5">
      <div class="flex items-center justify-between text-xs">
        <span class="font-bold text-slate-200 flex items-center gap-1.5">
          <span class="text-nordic-purple"><IconHome size={14} /></span>
          <span>Home Partition (/home)</span>
        </span>
        <span class="font-mono text-[11.5px] text-nordic-text-secondary">
          Home: {homeStats.free} Free / {homeStats.total} ({homeStats.usedPct}% Used)
        </span>
      </div>
      <div class="h-[7px] bg-[#162030] rounded overflow-hidden">
        <div
          class="h-full rounded bg-gradient-to-r from-[#a855f7] to-[#9333ea] transition-all duration-500 ease-out"
          style="width: {homeStats.pctVal}%;"
        ></div>
      </div>
    </div>
  {:else}
    <div class="flex flex-col gap-1.5 opacity-60">
      <div class="flex items-center justify-between text-xs">
        <span class="font-bold text-slate-400 flex items-center gap-1.5">
          <IconHome size={14} />
          <span>Home Partition (/home)</span>
        </span>
        <span class="font-mono text-[11.5px] text-nordic-text-muted">Unified with Root</span>
      </div>
      <div class="h-[7px] bg-[#162030] rounded overflow-hidden">
        <div class="h-full rounded bg-[#20293a]" style="width: 100%;"></div>
      </div>
    </div>
  {/if}

  <!-- Segmented Storage Filter -->
  <div class="flex flex-col items-end gap-1">
    <span class="text-[10px] uppercase font-bold tracking-wider text-nordic-text-muted">
      Target Storage Filter
    </span>
    <div class="flex bg-[#101622] border border-nordic-border rounded-md p-0.5">
      <button
        type="button"
        onclick={() => onFilterChange('all')}
        class="px-2.5 py-1 rounded text-[11px] font-mono font-semibold transition {currentFilter ===
        'all'
          ? 'bg-nordic-cyan text-[#041017] shadow-sm'
          : 'text-nordic-text-secondary hover:text-white'}"
      >
        ALL
      </button>
      <button
        type="button"
        onclick={() => onFilterChange('root')}
        class="px-2.5 py-1 rounded text-[11px] font-mono font-semibold transition {currentFilter ===
        'root'
          ? 'bg-nordic-cyan text-[#041017] shadow-sm'
          : 'text-nordic-text-secondary hover:text-white'}"
      >
        ROOT (/)
      </button>
      <button
        type="button"
        onclick={() => onFilterChange('home')}
        class="px-2.5 py-1 rounded text-[11px] font-mono font-semibold transition {currentFilter ===
        'home'
          ? 'bg-nordic-cyan text-[#041017] shadow-sm'
          : 'text-nordic-text-secondary hover:text-white'}"
      >
        HOME (~)
      </button>
    </div>
  </div>
</div>
