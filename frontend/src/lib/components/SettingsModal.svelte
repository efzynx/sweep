<script lang="ts">
  import type { AppConfig } from '../types';
  import { IconSettings, IconX, IconCheck, IconChevronDown } from '../icons';

  let {
    isOpen = false,
    config,
    onSave = async () => {},
    onClose = () => {},
  }: {
    isOpen: boolean;
    config: AppConfig | null;
    onSave: (cfg: AppConfig) => Promise<void>;
    onClose: () => void;
  } = $props();

  let dryRunDefault = $state(false);
  let snapshotSafeguard = $state(true);
  let logLevel = $state('info');

  let snapperDays = $state(7);
  let pacmanVersions = $state(2);
  let journalDays = $state(7);
  let devCacheDays = $state(30);

  let saving = $state(false);
  let saveSuccess = $state(false);

  // Sync state when config prop updates
  $effect(() => {
    if (config) {
      dryRunDefault = !!config.general?.dry_run_default;
      snapshotSafeguard = config.general?.snapshot_safeguard ?? true;
      logLevel = config.general?.log_level || 'info';

      snapperDays = config.retention?.snapper_min_age_days ?? 7;
      pacmanVersions = config.retention?.pacman_keep_versions ?? 2;
      journalDays = config.retention?.journal_vacuum_days ?? 7;
      devCacheDays = config.retention?.dev_cache_idle_days ?? 30;
    }
  });

  async function handleSave() {
    saving = true;
    saveSuccess = false;
    try {
      const updated: AppConfig = {
        general: {
          dry_run_default: dryRunDefault,
          snapshot_safeguard: snapshotSafeguard,
          log_level: logLevel,
        },
        retention: {
          snapper_min_age_days: Number(snapperDays) || 7,
          pacman_keep_versions: Number(pacmanVersions) || 2,
          journal_vacuum_days: Number(journalDays) || 7,
          dev_cache_idle_days: Number(devCacheDays) || 30,
        },
      };
      await onSave(updated);
      saveSuccess = true;
      setTimeout(() => {
        saveSuccess = false;
        onClose();
      }, 700);
    } catch (err) {
      console.error('Failed to save settings:', err);
    } finally {
      saving = false;
    }
  }
</script>

{#if isOpen}
  <div
    class="fixed inset-0 z-50 bg-[#04070c]/80 backdrop-blur-[4px] flex items-center justify-center p-4 select-none"
    role="dialog"
    aria-modal="true"
  >
    <div
      class="w-full max-w-[560px] bg-nordic-card border border-nordic-border rounded-xl shadow-2xl overflow-hidden flex flex-col"
    >
      <!-- Header -->
      <div
        class="px-5 py-3.5 bg-[#090e17] border-b border-nordic-border flex items-center justify-between"
      >
        <div class="flex items-center gap-2.5 text-sm font-bold text-white">
          <span class="text-nordic-cyan"><IconSettings size={17} /></span>
          <span>Preferences & Retention Settings</span>
        </div>
        <button
          type="button"
          class="text-nordic-text-muted hover:text-white transition-colors p-1"
          onclick={onClose}
        >
          <IconX size={16} />
        </button>
      </div>

      <!-- Body -->
      <div class="p-5 flex flex-col gap-5 max-h-[70vh] overflow-y-auto">
        <!-- Section: General Policies -->
        <div class="flex flex-col gap-3">
          <div class="text-[11px] font-bold tracking-wider uppercase text-nordic-text-muted border-b border-nordic-border/50 pb-1">
            General Policies
          </div>

          <label class="flex items-center justify-between p-3 rounded-lg bg-[#0c121d] border border-nordic-border/60 cursor-pointer hover:border-nordic-cyan/40 transition-colors">
            <div class="flex flex-col gap-0.5">
              <span class="text-xs font-semibold text-slate-200">Default to Dry-Run Mode</span>
              <span class="text-[11px] text-nordic-text-muted">Simulate clean operations without modifying disk by default</span>
            </div>
            <input
              type="checkbox"
              bind:checked={dryRunDefault}
              class="w-4 h-4 rounded bg-[#162030] border-nordic-border text-nordic-cyan focus:ring-0 cursor-pointer"
            />
          </label>

          <label class="flex items-center justify-between p-3 rounded-lg bg-[#0c121d] border border-nordic-border/60 cursor-pointer hover:border-nordic-cyan/40 transition-colors">
            <div class="flex flex-col gap-0.5">
              <span class="text-xs font-semibold text-slate-200">Snapper Safeguard Snapshots</span>
              <span class="text-[11px] text-nordic-text-muted">Create automated snapshot before executing destructive clean</span>
            </div>
            <input
              type="checkbox"
              bind:checked={snapshotSafeguard}
              class="w-4 h-4 rounded bg-[#162030] border-nordic-border text-nordic-cyan focus:ring-0 cursor-pointer"
            />
          </label>

          <div class="flex items-center justify-between p-3 rounded-lg bg-[#0c121d] border border-nordic-border/60">
            <div class="flex flex-col gap-0.5">
              <span class="text-xs font-semibold text-slate-200">Execution Log Level</span>
              <span class="text-[11px] text-nordic-text-muted">Verbosity of engine messages and diagnostics</span>
            </div>
            <div class="relative">
              <select
                bind:value={logLevel}
                class="appearance-none bg-[#162030] border border-nordic-border rounded px-3 py-1.5 pr-8 text-xs text-white focus:outline-none focus:border-nordic-cyan cursor-pointer [color-scheme:dark]"
              >
                <option value="info" class="bg-[#0c121d] text-slate-200">Info</option>
                <option value="debug" class="bg-[#0c121d] text-slate-200">Debug</option>
                <option value="warn" class="bg-[#0c121d] text-slate-200">Warn</option>
              </select>
              <div class="pointer-events-none absolute inset-y-0 right-0 flex items-center px-2 text-nordic-text-muted">
                <IconChevronDown size={13} />
              </div>
            </div>
          </div>
        </div>

        <!-- Section: Retention & Vacuum -->
        <div class="flex flex-col gap-3">
          <div class="text-[11px] font-bold tracking-wider uppercase text-nordic-text-muted border-b border-nordic-border/50 pb-1">
            Retention & Vacuum Limits
          </div>

          <div class="grid grid-cols-2 gap-3">
            <div class="p-3 rounded-lg bg-[#0c121d] border border-nordic-border/60 flex flex-col gap-1.5">
              <span class="text-xs font-semibold text-slate-200">Pacman Cache Keep</span>
              <span class="text-[10px] text-nordic-text-muted">Versions to preserve</span>
              <input
                type="number"
                min="1"
                max="10"
                bind:value={pacmanVersions}
                class="bg-[#162030] border border-nordic-border rounded px-2.5 py-1 text-xs text-white font-mono focus:outline-none focus:border-nordic-cyan mt-1"
              />
            </div>

            <div class="p-3 rounded-lg bg-[#0c121d] border border-nordic-border/60 flex flex-col gap-1.5">
              <span class="text-xs font-semibold text-slate-200">Journal Retention</span>
              <span class="text-[10px] text-nordic-text-muted">Days before log vacuum</span>
              <input
                type="number"
                min="1"
                max="365"
                bind:value={journalDays}
                class="bg-[#162030] border border-nordic-border rounded px-2.5 py-1 text-xs text-white font-mono focus:outline-none focus:border-nordic-cyan mt-1"
              />
            </div>

            <div class="p-3 rounded-lg bg-[#0c121d] border border-nordic-border/60 flex flex-col gap-1.5">
              <span class="text-xs font-semibold text-slate-200">Snapper Min Age</span>
              <span class="text-[10px] text-nordic-text-muted">Days before snapshot prune</span>
              <input
                type="number"
                min="1"
                max="365"
                bind:value={snapperDays}
                class="bg-[#162030] border border-nordic-border rounded px-2.5 py-1 text-xs text-white font-mono focus:outline-none focus:border-nordic-cyan mt-1"
              />
            </div>

            <div class="p-3 rounded-lg bg-[#0c121d] border border-nordic-border/60 flex flex-col gap-1.5">
              <span class="text-xs font-semibold text-slate-200">Dev Cache Idle</span>
              <span class="text-[10px] text-nordic-text-muted">Days of unaccessed cache</span>
              <input
                type="number"
                min="1"
                max="365"
                bind:value={devCacheDays}
                class="bg-[#162030] border border-nordic-border rounded px-2.5 py-1 text-xs text-white font-mono focus:outline-none focus:border-nordic-cyan mt-1"
              />
            </div>
          </div>
        </div>
      </div>

      <!-- Footer -->
      <div class="px-5 py-3.5 bg-[#090e17] border-t border-nordic-border flex items-center justify-between">
        <span class="text-[11px] text-nordic-text-muted">
          Persisted to <code class="text-slate-400">~/.config/sweep/config.yaml</code>
        </span>
        <div class="flex items-center gap-2.5">
          <button
            type="button"
            class="px-3 py-1.5 rounded text-xs font-medium text-slate-300 hover:text-white hover:bg-white/[0.04] transition-colors"
            onclick={onClose}
          >
            Cancel
          </button>
          <button
            type="button"
            disabled={saving}
            class="px-4 py-1.5 rounded text-xs font-semibold bg-nordic-cyan text-[#041017] hover:bg-nordic-cyan/90 transition-colors flex items-center gap-1.5 disabled:opacity-50"
            onclick={handleSave}
          >
            {#if saveSuccess}
              <IconCheck size={14} />
              <span>Saved!</span>
            {:else if saving}
              <span>Saving...</span>
            {:else}
              <span>Save Settings</span>
            {/if}
          </button>
        </div>
      </div>
    </div>
  </div>
{/if}
