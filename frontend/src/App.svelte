<script lang="ts">
  import { onMount } from 'svelte';
  import type {
    SystemStorageInfo,
    DistroInfo,
    ModuleSummary,
    ScanResult,
    StoragePartitionFilter,
    CleanableItem,
    ExecutionLogEntry,
  } from './lib/types';
  import { api } from './lib/api';
  import { formatBytes, formatDurationMs } from './lib/format';

  import Titlebar from './lib/components/Titlebar.svelte';
  import StorageBanner from './lib/components/StorageBanner.svelte';
  import Sidebar from './lib/components/Sidebar.svelte';
  import InspectorTable from './lib/components/InspectorTable.svelte';
  import BottomBar from './lib/components/BottomBar.svelte';
  import CustomPathModal from './lib/components/CustomPathModal.svelte';
  import CleanModal from './lib/components/CleanModal.svelte';
  import SettingsModal from './lib/components/SettingsModal.svelte';
  import type { AppConfig } from './lib/types';

  // App State
  let storage = $state<SystemStorageInfo | null>(null);
  let distro = $state<DistroInfo | null>(null);
  let modules = $state<ModuleSummary[]>([]);
  let scanResults = $state<Record<string, ScanResult>>({});
  let activeModuleId = $state<string>('');
  let storageFilter = $state<StoragePartitionFilter>('all');
  let searchQuery = $state<string>('');
  let scanning = $state<boolean>(false);
  let safeguardActive = $state<boolean>(true);
  let appConfig = $state<AppConfig | null>(null);
  let appVersion = $state<string>('1.0.1-beta');

  // Modals state
  let isCustomModalOpen = $state<boolean>(false);
  let isSettingsModalOpen = $state<boolean>(false);
  let isCleanModalOpen = $state<boolean>(false);
  let cleanModalDryRun = $state<boolean>(false);
  let cleanModalStatus = $state<'RUNNING' | 'COMPLETED' | 'FAILED'>('RUNNING');
  let cleanModalTotal = $state<number>(0);
  let cleanModalProcessed = $state<number>(0);
  let cleanModalFreed = $state<number>(0);
  let cleanModalLogs = $state<ExecutionLogEntry[]>([]);

  // Active module title
  const activeModule = $derived(
    modules.find((m) => m.id === activeModuleId) || null
  );

  const activeModuleTitle = $derived(
    activeModule ? activeModule.name : 'System Inspector'
  );

  // Items of active module
  const activeModuleItems = $derived(
    activeModuleId && scanResults[activeModuleId]
      ? scanResults[activeModuleId].items || []
      : []
  );

  // All selected items across all modules
  const allSelectedItems = $derived.by(() => {
    const list: CleanableItem[] = [];
    for (const modId of Object.keys(scanResults)) {
      const res = scanResults[modId];
      if (res && res.items) {
        for (const it of res.items) {
          if (it.selected) {
            list.push(it);
          }
        }
      }
    }
    return list;
  });

  const selectedStats = $derived.by(() => {
    let count = 0;
    let totalBytes = 0;
    let homeBytes = 0;
    let rootBytes = 0;

    for (const it of allSelectedItems) {
      count++;
      const sz = it.size || 0;
      totalBytes += sz;
      const target = (it.metadata?.storage_target || it.metadata?.storage || '').toLowerCase();
      if (target === 'home') {
        homeBytes += sz;
      } else {
        rootBytes += sz;
      }
    }

    return { count, totalBytes, homeBytes, rootBytes };
  });

  // Load system data and perform scan
  async function loadData() {
    scanning = true;
    try {
      const [storageData, distroData, modulesData, configData, versionData] = await Promise.all([
        api.getStorageInfo(),
        api.getDistroInfo(),
        api.getModules(),
        api.getConfig(),
        api.getAppVersion(),
      ]);

      storage = storageData;
      distro = distroData;
      modules = modulesData;
      appConfig = configData;
      if (versionData) {
        appVersion = versionData;
      }

      if (!activeModuleId && modulesData.length > 0) {
        activeModuleId = modulesData[0].id;
      }

      // Snapper safeguard active if snapper module is available and enabled in config
      const snapperMod = modulesData.find((m) => m.id === 'snapper');
      safeguardActive = !!snapperMod?.available && (configData.general?.snapshot_safeguard ?? true);

      // Scan all modules
      const scanMap = await api.scanAll();
      scanResults = scanMap;

      // If active module has no items, default to the first module that has items
      if (activeModuleId && (!scanMap[activeModuleId] || !scanMap[activeModuleId].items?.length)) {
        for (const m of modulesData) {
          if (scanMap[m.id]?.items?.length) {
            activeModuleId = m.id;
            break;
          }
        }
      }
    } catch (err) {
      console.error('Failed to load system scan data:', err);
    } finally {
      scanning = false;
    }
  }

  // Auto-switch active module when storageFilter changes so it doesn't get stuck on a hidden module
  $effect(() => {
    if (storageFilter !== 'all' && activeModuleId !== 'custom') {
      const currentRes = scanResults[activeModuleId];
      const hasCurrent = currentRes?.items?.some((it) => {
        const target = (it.metadata?.storage_target || it.metadata?.storage || '').toLowerCase();
        if (storageFilter === 'root') return target === 'root' || !target;
        if (storageFilter === 'home') return target === 'home';
        return true;
      });

      if (!hasCurrent) {
        for (const m of modules) {
          const res = scanResults[m.id];
          const has = res?.items?.some((it) => {
            const target = (it.metadata?.storage_target || it.metadata?.storage || '').toLowerCase();
            if (storageFilter === 'root') return target === 'root' || !target;
            if (storageFilter === 'home') return target === 'home';
            return true;
          });
          if (has) {
            activeModuleId = m.id;
            break;
          }
        }
      }
    }
  });

  // Save settings handler
  async function handleSaveSettings(cfg: AppConfig) {
    await api.updateConfig(cfg);
    appConfig = cfg;
    safeguardActive = cfg.general.snapshot_safeguard;
    await loadData();
  }

  onMount(() => {
    loadData();
  });

  // Toggle selection on a single item in active module
  function handleToggleItem(itemId: string) {
    if (!activeModuleId || !scanResults[activeModuleId]) return;
    const items = [...scanResults[activeModuleId].items];
    const idx = items.findIndex((it) => it.id === itemId);
    if (idx !== -1) {
      items[idx] = { ...items[idx], selected: !items[idx].selected };
      scanResults = {
        ...scanResults,
        [activeModuleId]: {
          ...scanResults[activeModuleId],
          items,
        },
      };
    }
  }

  // Toggle all items currently visible in the inspector table
  function handleToggleAllInView(selectAll: boolean) {
    if (!activeModuleId || !scanResults[activeModuleId]) return;
    const q = searchQuery.trim().toLowerCase();
    const updated = scanResults[activeModuleId].items.map((it) => {
      // Check if item is in view
      const target = (it.metadata?.storage_target || it.metadata?.storage || '').toLowerCase();
      let inPartition = true;
      if (storageFilter === 'root') inPartition = target === 'root' || !target;
      if (storageFilter === 'home') inPartition = target === 'home';

      let inSearch = true;
      if (q) {
        inSearch =
          it.name.toLowerCase().includes(q) ||
          Boolean(it.description && it.description.toLowerCase().includes(q)) ||
          Boolean(it.path && it.path.toLowerCase().includes(q));
      }

      if (inPartition && inSearch) {
        return { ...it, selected: selectAll };
      }
      return it;
    });

    scanResults = {
      ...scanResults,
      [activeModuleId]: {
        ...scanResults[activeModuleId],
        items: updated,
      },
    };
  }

  // Toggle all items in a module from the sidebar
  function handleToggleModuleSelection(modId: string, selectAll: boolean) {
    if (!scanResults[modId] || !scanResults[modId].items) return;
    const updated = scanResults[modId].items.map((it) => ({
      ...it,
      selected: selectAll,
    }));
    scanResults = {
      ...scanResults,
      [modId]: {
        ...scanResults[modId],
        items: updated,
      },
    };
  }

  // Save custom path rule
  async function handleSaveCustomRule(rule: {
    name: string;
    path: string;
    pattern: string;
    recursive: boolean;
    safety: string;
  }) {
    await api.addCustomRule(
      rule.name,
      rule.path,
      rule.pattern,
      rule.recursive,
      rule.safety
    );
    // Auto-rescan
    await loadData();
    // Switch to custom module to show results
    activeModuleId = 'custom';
  }

  // Helper to add log entries
  function addLog(level: ExecutionLogEntry['level'], message: string) {
    cleanModalLogs = [
      ...cleanModalLogs,
      {
        id: Math.random().toString(36).substring(2, 9),
        timestamp: new Date().toLocaleTimeString(),
        level,
        message,
      },
    ];
  }

  // Execute Dry-Run or Real Cleaning
  async function executeClean(dryRun: boolean) {
    const itemsToClean = allSelectedItems;
    if (itemsToClean.length === 0) return;

    cleanModalDryRun = dryRun;
    cleanModalStatus = 'RUNNING';
    cleanModalTotal = itemsToClean.length;
    cleanModalProcessed = 0;
    cleanModalFreed = 0;
    cleanModalLogs = [];
    isCleanModalOpen = true;

    // Distinct modules affected
    const distinctModules = new Set(
      itemsToClean.map((it) => it.metadata?.module || 'system')
    );

    addLog(
      'INIT',
      `Starting ${dryRun ? 'Dry-Run Simulation' : 'Cleanup Operation'} across ${distinctModules.size} modules (${itemsToClean.length} items queued)...`
    );

    if (safeguardActive && !dryRun) {
      addLog('SNAP', 'Requesting Snapper pre-clean safeguard snapshot...');
    }

    try {
      // Call Go bridge with privilege elevation support
      const result = await api.cleanItems(itemsToClean, dryRun, safeguardActive);

      // Report any errors returned from engine
      if (result.errors && result.errors.length > 0) {
        for (const errText of result.errors) {
          addLog('ERROR', errText);
        }
      }

      // Report items that were successfully cleaned
      if (result.cleaned_items && result.cleaned_items.length > 0) {
        for (const it of result.cleaned_items) {
          addLog(
            'CLEAN',
            `${dryRun ? '[DRY-RUN] Verified' : 'Cleaned'} ${it.path || it.name} (${formatBytes(it.size || 0)})`
          );
        }
      }

      cleanModalProcessed = result.cleaned_items?.length || 0;
      cleanModalFreed = result.freed_bytes || 0;

      const durationMs = result.duration_ms ?? 0;

      if (!result.success || (result.errors && result.errors.length > 0 && cleanModalProcessed === 0)) {
        cleanModalStatus = 'FAILED';
        addLog(
          'ERROR',
          `Operation failed: items could not be removed. Check system permissions / root authorization.`
        );
      } else if (result.errors && result.errors.length > 0) {
        cleanModalStatus = 'COMPLETED'; // partial completion
        addLog(
          'WARN',
          `Finished with warnings: ${cleanModalProcessed}/${itemsToClean.length} items cleaned in ${durationMs}ms (${formatBytes(cleanModalFreed)} reclaimed)`
        );
      } else {
        cleanModalStatus = 'COMPLETED';
        addLog(
          'DONE',
          `${dryRun ? 'Simulation' : 'Cleanup'} finished successfully in ${durationMs}ms (${formatBytes(cleanModalFreed)} reclaimed)`
        );
      }

      // Refresh storage metrics and scan results after real clean
      if (!dryRun && result.cleaned_items && result.cleaned_items.length > 0) {
        api.getStorageInfo().then((s) => (storage = s));
        const cleanedIds = new Set(result.cleaned_items.map((it) => it.id));
        const nextResults: Record<string, ScanResult> = {};
        for (const [mid, res] of Object.entries(scanResults)) {
          const remaining = res.items.filter((it) => !cleanedIds.has(it.id));
          nextResults[mid] = {
            ...res,
            items: remaining,
            total_size: remaining.reduce((acc, it) => acc + (it.size || 0), 0),
          };
        }
        scanResults = nextResults;
      }
    } catch (err: any) {
      cleanModalStatus = 'FAILED';
      addLog('ERROR', `Execution aborted: ${err?.message || String(err)}`);
    }
  }
</script>

<div class="h-screen w-screen flex flex-col bg-nordic-window overflow-hidden">
  <!-- Titlebar -->
  <Titlebar
    {distro}
    {appVersion}
    {safeguardActive}
    {scanning}
    onRefresh={loadData}
    onOpenSettings={() => (isSettingsModalOpen = true)}
  />

  <!-- Storage Telemetry Banner -->
  <StorageBanner
    {storage}
    currentFilter={storageFilter}
    onFilterChange={(f) => (storageFilter = f)}
  />

  <!-- Main Split Workspace -->
  <div class="flex-1 flex overflow-hidden">
    <!-- Subsystems Sidebar -->
    <Sidebar
      {modules}
      {scanResults}
      {activeModuleId}
      {storageFilter}
      onSelectModule={(id) => (activeModuleId = id)}
      onToggleModuleSelection={handleToggleModuleSelection}
    />

    <!-- Main Content / Table Inspector -->
    <InspectorTable
      moduleTitle={activeModuleTitle}
      items={activeModuleItems}
      {storageFilter}
      bind:searchQuery
      onToggleItem={handleToggleItem}
      onToggleAllInView={handleToggleAllInView}
      onOpenCustomModal={() => (isCustomModalOpen = true)}
    />
  </div>

  <!-- Bottom Action Bar -->
  <BottomBar
    selectedCount={selectedStats.count}
    selectedBytes={selectedStats.totalBytes}
    homeBytes={selectedStats.homeBytes}
    rootBytes={selectedStats.rootBytes}
    disabled={scanning}
    onDryRun={() => executeClean(true)}
    onClean={() => executeClean(false)}
  />

  <!-- In-App Custom Path Modal Form -->
  <CustomPathModal
    isOpen={isCustomModalOpen}
    onClose={() => (isCustomModalOpen = false)}
    onSave={handleSaveCustomRule}
  />

  <!-- In-App Settings & Retention Modal -->
  <SettingsModal
    isOpen={isSettingsModalOpen}
    config={appConfig}
    onClose={() => (isSettingsModalOpen = false)}
    onSave={handleSaveSettings}
  />

  <!-- Live Clean & Simulation Execution Modal -->
  <CleanModal
    isOpen={isCleanModalOpen}
    isDryRun={cleanModalDryRun}
    status={cleanModalStatus}
    totalItems={cleanModalTotal}
    processedItems={cleanModalProcessed}
    freedBytes={cleanModalFreed}
    logs={cleanModalLogs}
    onClose={() => {
      isCleanModalOpen = false;
      loadData();
    }}
  />
</div>
