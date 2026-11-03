<script lang="ts">
  import type { CleanableItem, StoragePartitionFilter } from '../types';
  import { formatBytes } from '../format';
  import { IconSearch, IconPlus, IconCheck } from '../icons';

  let {
    moduleTitle = '',
    items = [],
    storageFilter = 'all',
    searchQuery = $bindable(''),
    onToggleItem = () => {},
    onToggleAllInView = () => {},
    onOpenCustomModal = () => {},
  }: {
    moduleTitle: string;
    items: CleanableItem[];
    storageFilter: StoragePartitionFilter;
    searchQuery: string;
    onToggleItem: (itemId: string) => void;
    onToggleAllInView: (selectAll: boolean) => void;
    onOpenCustomModal: () => void;
  } = $props();

  // Filter items by storage partition AND search query
  const filteredItems = $derived.by(() => {
    let result = items;

    // Filter by storage partition
    if (storageFilter !== 'all') {
      result = result.filter((it) => {
        const target = (it.metadata?.storage_target || it.metadata?.storage || '').toLowerCase();
        if (storageFilter === 'root') return target === 'root' || !target;
        if (storageFilter === 'home') return target === 'home';
        return true;
      });
    }

    // Filter by search query
    const q = searchQuery.trim().toLowerCase();
    if (q) {
      result = result.filter(
        (it) =>
          it.name.toLowerCase().includes(q) ||
          Boolean(it.description && it.description.toLowerCase().includes(q)) ||
          Boolean(it.path && it.path.toLowerCase().includes(q))
      );
    }

    return result;
  });

  const totalFilteredSize = $derived(
    filteredItems.reduce((acc, it) => acc + (it.size || 0), 0)
  );

  const allVisibleSelected = $derived(
    filteredItems.length > 0 && filteredItems.every((it) => it.selected)
  );

  const someVisibleSelected = $derived(
    filteredItems.some((it) => it.selected) && !allVisibleSelected
  );
</script>

<div class="flex-1 flex flex-col bg-nordic-window overflow-hidden">
  <!-- Content Toolbar -->
  <div
    class="px-5 py-3 border-b border-nordic-border flex flex-wrap items-center justify-between gap-3 shrink-0"
  >
    <div class="flex items-baseline gap-2.5">
      <h2 class="text-base font-bold text-white">{moduleTitle || 'Inspector'}</h2>
      <span class="text-xs font-mono text-nordic-text-muted">
        {filteredItems.length} {filteredItems.length === 1 ? 'target' : 'targets'} detected ({formatBytes(totalFilteredSize)})
      </span>
    </div>

    <div class="flex items-center gap-2.5">
      <!-- Search Input -->
      <div
        class="flex items-center gap-2 bg-[#080c13] border border-nordic-border rounded-md px-2.5 py-1.5 w-52 focus-within:border-nordic-cyan transition-colors"
      >
        <span class="text-nordic-text-muted"><IconSearch size={13} /></span>
        <input
          type="text"
          placeholder="Filter items..."
          bind:value={searchQuery}
          class="bg-transparent border-none text-xs text-white placeholder-nordic-text-muted outline-none w-full font-sans"
        />
        {#if searchQuery}
          <button
            type="button"
            onclick={() => (searchQuery = '')}
            class="text-nordic-text-muted hover:text-white text-xs"
          >
            ✕
          </button>
        {/if}
      </div>

      <!-- Add Custom Path Button -->
      <button
        type="button"
        onclick={onOpenCustomModal}
        class="bg-[#111723] hover:bg-[#162030] border border-nordic-border text-slate-200 px-3 py-1.5 rounded-md text-xs font-semibold flex items-center gap-1.5 transition"
      >
        <IconPlus size={13} />
        <span>Add Custom Path</span>
      </button>
    </div>
  </div>

  <!-- Table Scroll Container -->
  <div class="flex-1 overflow-y-auto">
    {#if filteredItems.length === 0}
      <!-- Empty State -->
      <div class="h-full flex flex-col items-center justify-center p-8 text-center text-nordic-text-muted select-none">
        <div class="w-12 h-12 rounded-full bg-[#121824] border border-nordic-border flex items-center justify-center text-nordic-text-muted mb-3">
          <IconSearch size={20} />
        </div>
        {#if searchQuery}
          <p class="text-sm font-medium text-slate-300">No targets match filter</p>
          <p class="text-xs text-nordic-text-muted mt-1">No items found matching "{searchQuery}"</p>
        {:else}
          <p class="text-sm font-medium text-slate-300">Clean State</p>
          <p class="text-xs text-nordic-text-muted mt-1">No reclaimable files or cache entries found in this subsystem.</p>
        {/if}
      </div>
    {:else}
      <table class="w-full border-collapse text-xs select-none">
        <thead>
          <tr class="bg-[#090e17] sticky top-0 z-10 border-b border-nordic-border text-nordic-text-muted font-semibold text-[11px]">
            <th class="w-10 px-4 py-2 text-left">
              <button
                type="button"
                class="w-[15px] h-[15px] rounded border flex items-center justify-center transition-colors {allVisibleSelected
                  ? 'bg-nordic-cyan border-nordic-cyan text-[#041017]'
                  : someVisibleSelected
                    ? 'bg-nordic-cyan/30 border-nordic-cyan text-nordic-cyan'
                    : 'bg-[#0f172a] border-[#334155]'}"
                onclick={() => onToggleAllInView(!allVisibleSelected)}
                title={allVisibleSelected ? 'Deselect all' : 'Select all'}
              >
                {#if allVisibleSelected}
                  <IconCheck size={11} />
                {:else if someVisibleSelected}
                  <div class="w-1.5 h-1.5 bg-nordic-cyan rounded-sm"></div>
                {/if}
              </button>
            </th>
            <th class="px-4 py-2 text-left">Cleanable Target</th>
            <th class="px-4 py-2 text-left">Location</th>
            <th class="px-4 py-2 text-left">Size</th>
            <th class="px-4 py-2 text-left">Safety Level</th>
            <th class="px-4 py-2 text-left">Target Path</th>
          </tr>
        </thead>
        <tbody class="divide-y divide-[#1c2536]/40">
          {#each filteredItems as item (item.id)}
            {@const isHome = (item.metadata?.storage_target || item.metadata?.storage || '').toLowerCase() === 'home'}
            {@const isRoot = !isHome}
            {@const isSafe = (item.safety || 'SAFE').toUpperCase() === 'SAFE'}
            {@const isCaution = (item.safety || '').toUpperCase() === 'CAUTION'}

            <tr
              class="transition-colors cursor-pointer hover:bg-white/[0.02] {item.selected ? 'bg-nordic-cyan/[0.04]' : ''}"
              onclick={() => onToggleItem(item.id)}
            >
              <!-- Checkbox -->
              <td class="px-4 py-2.5">
                <button
                  type="button"
                  class="w-[15px] h-[15px] rounded border flex items-center justify-center transition-colors {item.selected
                    ? 'bg-nordic-cyan border-nordic-cyan text-[#041017]'
                    : 'bg-[#0f172a] border-[#334155]'}"
                  onclick={(e) => {
                    e.stopPropagation();
                    onToggleItem(item.id);
                  }}
                >
                  {#if item.selected}
                    <IconCheck size={11} />
                  {/if}
                </button>
              </td>

              <!-- Target Name & Subtext -->
              <td class="px-4 py-2.5">
                <div class="flex flex-col gap-0.5">
                  <span class="font-semibold text-white">{item.name}</span>
                  {#if item.description}
                    <span class="text-[10.5px] font-mono text-nordic-text-muted truncate max-w-md">
                      {item.description}
                    </span>
                  {/if}
                </div>
              </td>

              <!-- Location Badge -->
              <td class="px-4 py-2.5">
                {#if isHome}
                  <span
                    class="inline-flex items-center px-2 py-0.5 rounded text-[10.5px] font-bold font-mono bg-nordic-purple-dim text-nordic-purple border border-nordic-purple/30"
                  >
                    [~] HOME
                  </span>
                {:else}
                  <span
                    class="inline-flex items-center px-2 py-0.5 rounded text-[10.5px] font-bold font-mono bg-nordic-cyan-dim text-nordic-cyan border border-nordic-cyan/30"
                  >
                    [/] ROOT
                  </span>
                {/if}
              </td>

              <!-- Size -->
              <td class="px-4 py-2.5 font-mono font-bold {item.selected ? 'text-white' : 'text-slate-400'}">
                {formatBytes(item.size)}
              </td>

              <!-- Safety Badge -->
              <td class="px-4 py-2.5">
                {#if isSafe}
                  <span
                    class="inline-flex items-center px-2 py-0.5 rounded text-[10.5px] font-bold font-mono bg-nordic-green-dim text-nordic-green border border-nordic-green/30"
                  >
                    SAFE
                  </span>
                {:else if isCaution}
                  <span
                    class="inline-flex items-center px-2 py-0.5 rounded text-[10.5px] font-bold font-mono bg-nordic-amber-dim text-nordic-amber border border-nordic-amber/30"
                  >
                    CAUTION
                  </span>
                {:else}
                  <span
                    class="inline-flex items-center px-2 py-0.5 rounded text-[10.5px] font-bold font-mono bg-nordic-red-dim text-nordic-red border border-nordic-red/30"
                  >
                    HIGH
                  </span>
                {/if}
              </td>

              <!-- Path -->
              <td class="px-4 py-2.5 font-mono text-[11px] text-nordic-text-muted truncate max-w-xs" title={item.path}>
                {item.path || '—'}
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    {/if}
  </div>
</div>
