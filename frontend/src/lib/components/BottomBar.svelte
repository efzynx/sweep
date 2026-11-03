<script lang="ts">
  import { formatBytes } from '../format';

  let {
    selectedCount = 0,
    selectedBytes = 0,
    homeBytes = 0,
    rootBytes = 0,
    disabled = false,
    onDryRun = () => {},
    onClean = () => {},
  }: {
    selectedCount: number;
    selectedBytes: number;
    homeBytes: number;
    rootBytes: number;
    disabled?: boolean;
    onDryRun: () => void;
    onClean: () => void;
  } = $props();
</script>

<footer
  class="h-14 bg-[#080c13] border-t border-nordic-border px-5 flex items-center justify-between select-none shrink-0"
>
  <!-- Selection Summary Left -->
  <div class="flex items-center gap-4">
    <div class="text-[13px] text-slate-200">
      Selected Space:
      <strong class="text-[15px] text-nordic-cyan font-mono ml-1">
        {formatBytes(selectedBytes)}
      </strong>
      <span class="text-nordic-text-muted text-xs font-mono ml-1">
        ({selectedCount} {selectedCount === 1 ? 'item' : 'items'})
      </span>
    </div>

    <!-- Storage Breakdown Pills -->
    <div class="hidden sm:flex items-center gap-2">
      <div
        class="text-[11px] px-2 py-0.5 rounded bg-[#111723] border border-nordic-border text-nordic-text-secondary"
      >
        Home (~): <span class="font-bold text-white font-mono">{formatBytes(homeBytes)}</span>
      </div>
      <div
        class="text-[11px] px-2 py-0.5 rounded bg-[#111723] border border-nordic-border text-nordic-text-secondary"
      >
        Root (/): <span class="font-bold text-white font-mono">{formatBytes(rootBytes)}</span>
      </div>
    </div>
  </div>

  <!-- Bottom Actions Right -->
  <div class="flex items-center gap-3">
    <button
      type="button"
      onclick={onDryRun}
      disabled={disabled || selectedCount === 0}
      class="bg-[#151d2c] hover:bg-[#1a2538] border border-nordic-border text-slate-200 px-4 py-2 rounded-md text-[12.5px] font-semibold transition disabled:opacity-40 disabled:cursor-not-allowed"
    >
      Simulate Dry-Run
    </button>
    <button
      type="button"
      onclick={onClean}
      disabled={disabled || selectedCount === 0}
      class="bg-nordic-cyan hover:bg-[#22d3ee] text-[#041017] px-5 py-2 rounded-md text-[12.5px] font-bold shadow-glow hover:shadow-glow-lg transition disabled:opacity-40 disabled:cursor-not-allowed disabled:shadow-none"
    >
      Clean Selected Space
    </button>
  </div>
</footer>
