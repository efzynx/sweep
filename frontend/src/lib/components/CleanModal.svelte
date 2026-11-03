<script lang="ts">
  import type { ExecutionLogEntry } from '../types';
  import { formatBytes } from '../format';
  import { IconTerminal, IconCheck } from '../icons';

  let {
    isOpen = false,
    isDryRun = false,
    status = 'RUNNING', // 'RUNNING' | 'COMPLETED' | 'FAILED'
    totalItems = 0,
    processedItems = 0,
    freedBytes = 0,
    logs = [],
    onClose = () => {},
  }: {
    isOpen: boolean;
    isDryRun?: boolean;
    status: 'RUNNING' | 'COMPLETED' | 'FAILED';
    totalItems: number;
    processedItems: number;
    freedBytes: number;
    logs: ExecutionLogEntry[];
    onClose: () => void;
  } = $props();

  const progressPercent = $derived.by(() => {
    if (totalItems <= 0) return status === 'COMPLETED' ? 100 : 0;
    const pct = Math.round((processedItems / totalItems) * 100);
    return Math.min(100, Math.max(0, pct));
  });

  let logContainer: HTMLDivElement | null = $state(null);

  $effect(() => {
    if (logs.length && logContainer) {
      logContainer.scrollTop = logContainer.scrollHeight;
    }
  });
</script>

{#if isOpen}
  <div
    class="fixed inset-0 z-50 bg-[#04070c]/80 backdrop-blur-[4px] flex items-center justify-center p-4 select-none"
    role="dialog"
    aria-modal="true"
  >
    <div
      class="w-full max-w-[640px] bg-nordic-card border border-nordic-border rounded-xl shadow-2xl overflow-hidden flex flex-col"
    >
      <!-- Header -->
      <div
        class="px-5 py-3.5 bg-[#090e17] border-b border-nordic-border flex items-center justify-between"
      >
        <div class="flex items-center gap-2.5 text-sm font-bold text-white">
          <span class="text-nordic-cyan"><IconTerminal size={16} /></span>
          <span>{isDryRun ? 'Simulating System Cleanup (Dry-Run)' : 'Executing System Cleanup'}</span>
        </div>

        {#if status === 'RUNNING'}
          <span
            class="px-2.5 py-0.5 rounded text-[11px] font-mono font-bold bg-nordic-cyan-dim text-nordic-cyan border border-nordic-cyan/30 animate-pulse"
          >
            IN PROGRESS
          </span>
        {:else if status === 'COMPLETED'}
          <span
            class="px-2.5 py-0.5 rounded text-[11px] font-mono font-bold bg-nordic-green-dim text-nordic-green border border-nordic-green/30"
          >
            COMPLETED
          </span>
        {:else}
          <span
            class="px-2.5 py-0.5 rounded text-[11px] font-mono font-bold bg-nordic-red-dim text-nordic-red border border-nordic-red/30"
          >
            FAILED
          </span>
        {/if}
      </div>

      <!-- Body -->
      <div class="p-5 flex flex-col gap-4">
        <!-- Progress Counter -->
        <div class="flex items-center justify-between text-xs">
          <span class="text-slate-300">
            Progress: <strong class="text-white font-mono">{processedItems} / {totalItems} items {isDryRun ? 'simulated' : 'cleaned'}</strong>
          </span>
          <span class="text-nordic-cyan font-bold font-mono text-sm">
            {formatBytes(freedBytes)} {isDryRun ? 'Simulated' : 'Freed'}
          </span>
        </div>

        <!-- Progress Track -->
        <div class="h-2.5 bg-[#162030] rounded-full overflow-hidden">
          <div
            class="h-full rounded-full transition-all duration-300 ease-out {status === 'FAILED'
              ? 'bg-nordic-red'
              : status === 'COMPLETED'
                ? 'bg-nordic-green'
                : 'bg-nordic-cyan'}"
            style="width: {progressPercent}%;"
          ></div>
        </div>

        <!-- Monospace Console Log Viewer -->
        <div
          bind:this={logContainer}
          class="bg-[#06090e] border border-nordic-border rounded-lg p-3 font-mono text-[11px] h-48 overflow-y-auto leading-relaxed select-text"
        >
          {#if logs.length === 0}
            <div class="text-nordic-text-muted">Awaiting execution output...</div>
          {:else}
            {#each logs as log (log.id)}
              <div class="py-0.5">
                {#if log.level === 'INIT'}
                  <span class="text-slate-400 font-bold">[INIT]</span>
                  <span class="text-slate-400"> {log.message}</span>
                {:else if log.level === 'SNAP'}
                  <span class="text-nordic-green font-bold">[SNAP]</span>
                  <span class="text-nordic-green"> {log.message}</span>
                {:else if log.level === 'CLEAN'}
                  <span class="text-nordic-cyan font-bold">[CLEAN]</span>
                  <span class="text-slate-300"> {log.message}</span>
                {:else if log.level === 'WARN'}
                  <span class="text-nordic-amber font-bold">[WARN]</span>
                  <span class="text-nordic-amber"> {log.message}</span>
                {:else if log.level === 'ERROR'}
                  <span class="text-nordic-red font-bold">[ERR]</span>
                  <span class="text-nordic-red"> {log.message}</span>
                {:else if log.level === 'DONE'}
                  <span class="text-nordic-green font-bold">[DONE]</span>
                  <span class="text-nordic-green font-bold"> {log.message}</span>
                {:else}
                  <span class="text-nordic-text-muted"> {log.message}</span>
                {/if}
              </div>
            {/each}
          {/if}
        </div>
      </div>

      <!-- Footer -->
      <div
        class="px-5 py-3 bg-[#090e17] border-t border-nordic-border flex justify-end"
      >
        <button
          type="button"
          onclick={onClose}
          disabled={status === 'RUNNING'}
          class="bg-nordic-cyan hover:bg-[#22d3ee] text-[#041017] px-5 py-2 rounded-md text-xs font-bold shadow-glow transition disabled:opacity-40 disabled:cursor-not-allowed"
        >
          {status === 'RUNNING' ? 'Cleaning In Progress...' : 'Back to Dashboard'}
        </button>
      </div>
    </div>
  </div>
{/if}
