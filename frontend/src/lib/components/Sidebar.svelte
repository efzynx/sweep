<script lang="ts">
  import type { ModuleSummary, ScanResult, StoragePartitionFilter } from '../types';
  import { formatBytes } from '../format';
  import { IconCheck } from '../icons';

  let {
    modules = [],
    scanResults = {},
    activeModuleId = '',
    storageFilter = 'all',
    onSelectModule = () => {},
    onToggleModuleSelection = () => {},
  }: {
    modules: ModuleSummary[];
    scanResults: Record<string, ScanResult>;
    activeModuleId: string;
    storageFilter: StoragePartitionFilter;
    onSelectModule: (id: string) => void;
    onToggleModuleSelection: (id: string, selectAll: boolean) => void;
  } = $props();

  // Known category order
  const categoryOrder = [
    'Developer Caches',
    'Package Managers',
    'Containers & System',
    'Custom Rules',
  ];

  // Group modules by category
  const groupedModules = $derived.by(() => {
    const groups: { name: string; items: ModuleSummary[] }[] = [];
    const map = new Map<string, ModuleSummary[]>();

    for (const m of modules) {
      const cat = m.category || 'Containers & System';
      if (!map.has(cat)) {
        map.set(cat, []);
      }
      map.get(cat)!.push(m);
    }

    for (const cat of categoryOrder) {
      if (map.has(cat)) {
        groups.push({ name: cat, items: map.get(cat)! });
        map.delete(cat);
      }
    }

    // Any remaining categories
    for (const [cat, items] of map.entries()) {
      groups.push({ name: cat, items });
    }

    return groups;
  });

  // Calculate size and selection status per module taking storageFilter into account
  function getModuleStats(modId: string) {
    const res = scanResults[modId];
    if (!res || !res.items || res.items.length === 0) {
      return { totalSize: 0, allSelected: false, someSelected: false, count: 0 };
    }

    const items = res.items.filter((it) => {
      if (storageFilter === 'all') return true;
      const target = (it.metadata?.storage_target || it.metadata?.storage || '').toLowerCase();
      if (storageFilter === 'root') return target === 'root' || !target;
      if (storageFilter === 'home') return target === 'home';
      return true;
    });

    if (items.length === 0) {
      return { totalSize: 0, allSelected: false, someSelected: false, count: 0 };
    }

    const totalSize = items.reduce((acc, it) => acc + (it.size || 0), 0);
    const selectedCount = items.filter((it) => it.selected).length;

    return {
      totalSize,
      allSelected: selectedCount === items.length && items.length > 0,
      someSelected: selectedCount > 0 && selectedCount < items.length,
      count: items.length,
    };
  }
</script>

<aside
  class="w-72 bg-nordic-sidebar border-r border-nordic-border flex flex-col overflow-y-auto select-none shrink-0"
>
  {#each groupedModules as group}
    <div class="pt-3 pb-1 px-4 text-[11px] font-bold tracking-wider uppercase text-nordic-text-muted">
      {group.name}
    </div>

    {#each group.items as mod}
      {@const stats = getModuleStats(mod.id)}
      {@const isActive = activeModuleId === mod.id}

      <div
        class="px-4 py-2 flex items-center justify-between cursor-pointer border-l-[3px] transition-colors {isActive
          ? 'bg-[#111723] border-nordic-cyan'
          : 'border-transparent hover:bg-white/[0.03]'}"
        onclick={() => onSelectModule(mod.id)}
        role="button"
        tabindex="0"
        onkeydown={(e) => e.key === 'Enter' && onSelectModule(mod.id)}
      >
        <div class="flex items-center gap-2.5 min-w-0">
          <!-- Checkbox -->
          <button
            type="button"
            class="w-[15px] h-[15px] rounded border flex items-center justify-center transition-colors shrink-0 {stats.allSelected
              ? 'bg-nordic-cyan border-nordic-cyan text-[#041017]'
              : stats.someSelected
                ? 'bg-nordic-cyan/30 border-nordic-cyan text-nordic-cyan'
                : 'bg-[#0f172a] border-[#334155]'}"
            onclick={(e) => {
              e.stopPropagation();
              onToggleModuleSelection(mod.id, !stats.allSelected);
            }}
            title={stats.allSelected ? 'Deselect all items' : 'Select all items'}
          >
            {#if stats.allSelected}
              <IconCheck size={11} />
            {:else if stats.someSelected}
              <div class="w-1.5 h-1.5 bg-nordic-cyan rounded-sm"></div>
            {/if}
          </button>

          <!-- Module Name -->
          <span
            class="text-[13px] truncate {isActive
              ? 'text-white font-semibold'
              : 'text-slate-300 font-medium'}"
          >
            {mod.name}
          </span>
        </div>

        <!-- Reclaimable Badge -->
        <span
          class="font-mono text-[11.5px] ml-2 shrink-0 {stats.totalSize > 0
            ? 'text-nordic-cyan font-semibold'
            : 'text-nordic-text-muted'}"
        >
          {formatBytes(stats.totalSize)}
        </span>
      </div>
    {/each}
  {/each}
</aside>
