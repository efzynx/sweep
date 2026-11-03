<script lang="ts">
  import { IconPlus, IconX, IconCheck, IconChevronDown } from '../icons';

  let {
    isOpen = false,
    onClose = () => {},
    onSave = () => {},
  }: {
    isOpen: boolean;
    onClose: () => void;
    onSave: (rule: {
      name: string;
      path: string;
      pattern: string;
      recursive: boolean;
      safety: string;
    }) => Promise<void> | void;
  } = $props();

  let name = $state('Temporary Build Artifacts');
  let path = $state('~/tmp/builds');
  let pattern = $state('*.tmp');
  let recursive = $state(true);
  let safety = $state('SAFE');
  let saving = $state(false);
  let errorMessage = $state('');

  function handleSubmit() {
    errorMessage = '';
    const trimmedName = name.trim();
    const trimmedPath = path.trim();
    const trimmedPattern = pattern.trim();

    if (!trimmedName) {
      errorMessage = 'Rule name cannot be empty';
      return;
    }
    if (!trimmedPath) {
      errorMessage = 'Directory path cannot be empty';
      return;
    }

    saving = true;
    try {
      Promise.resolve(
        onSave({
          name: trimmedName,
          path: trimmedPath,
          pattern: trimmedPattern || '*',
          recursive,
          safety,
        })
      )
        .then(() => {
          saving = false;
          onClose();
        })
        .catch((err: any) => {
          saving = false;
          errorMessage = err?.message || String(err);
        });
    } catch (err: any) {
      saving = false;
      errorMessage = err?.message || String(err);
    }
  }
</script>

{#if isOpen}
  <div
    class="fixed inset-0 z-50 bg-[#04070c]/75 backdrop-blur-[4px] flex items-center justify-center p-4 select-none"
    role="dialog"
    aria-modal="true"
  >
    <div
      class="w-full max-w-[520px] bg-nordic-card border border-nordic-border rounded-xl shadow-2xl overflow-hidden"
    >
      <!-- Header -->
      <div
        class="px-4 py-3 bg-[#090e17] border-b border-nordic-border flex items-center justify-between"
      >
        <div class="flex items-center gap-2 text-sm font-bold text-white">
          <span class="text-nordic-cyan"><IconPlus size={15} /></span>
          <span>Add Custom Scan Path</span>
        </div>
        <button
          type="button"
          onclick={onClose}
          class="p-1 rounded text-nordic-text-muted hover:text-white hover:bg-white/5 transition"
        >
          <IconX size={15} />
        </button>
      </div>

      <!-- Body -->
      <form onsubmit={(e) => { e.preventDefault(); handleSubmit(); }} class="p-4 flex flex-col gap-3.5 text-xs">
        {#if errorMessage}
          <div class="p-2.5 rounded bg-nordic-red-dim border border-nordic-red/30 text-nordic-red font-mono text-[11px]">
            {errorMessage}
          </div>
        {/if}

        <div class="flex flex-col gap-1.5">
          <label for="rule-name" class="font-semibold text-nordic-text-secondary text-[11.5px]">Rule Name</label>
          <input
            id="rule-name"
            type="text"
            bind:value={name}
            placeholder="e.g. Node Temporary Cache"
            class="bg-[#080c13] border border-nordic-border rounded-md px-3 py-2 text-white font-sans text-xs focus:border-nordic-cyan outline-none transition"
          />
        </div>

        <div class="flex flex-col gap-1.5">
          <label for="dir-path" class="font-semibold text-nordic-text-secondary text-[11.5px]">Directory Path</label>
          <input
            id="dir-path"
            type="text"
            bind:value={path}
            placeholder="e.g. ~/tmp/builds or /var/tmp"
            class="bg-[#080c13] border border-nordic-border rounded-md px-3 py-2 text-white font-mono text-xs focus:border-nordic-cyan outline-none transition"
          />
        </div>

        <div class="grid grid-cols-2 gap-3">
          <div class="flex flex-col gap-1.5">
            <label for="file-pattern" class="font-semibold text-nordic-text-secondary text-[11.5px]">File Pattern (Glob)</label>
            <input
              id="file-pattern"
              type="text"
              bind:value={pattern}
              placeholder="e.g. *.tmp, *.log"
              class="bg-[#080c13] border border-nordic-border rounded-md px-3 py-2 text-white font-mono text-xs focus:border-nordic-cyan outline-none transition"
            />
          </div>

          <div class="flex flex-col gap-1.5">
            <label for="safety-level" class="font-semibold text-nordic-text-secondary text-[11.5px]">Safety Level</label>
            <div class="relative">
              <select
                id="safety-level"
                bind:value={safety}
                class="w-full appearance-none bg-[#080c13] border border-nordic-border rounded-md px-3 py-2 pr-8 text-white font-sans text-xs focus:border-nordic-cyan outline-none transition cursor-pointer [color-scheme:dark]"
              >
                <option value="SAFE" class="bg-[#0c121d] text-slate-200">SAFE (Recommended)</option>
                <option value="CAUTION" class="bg-[#0c121d] text-slate-200">CAUTION</option>
                <option value="HIGH" class="bg-[#0c121d] text-slate-200">HIGH</option>
              </select>
              <div class="pointer-events-none absolute inset-y-0 right-0 flex items-center px-2.5 text-nordic-text-muted">
                <IconChevronDown size={14} />
              </div>
            </div>
          </div>
        </div>

        <!-- Recursive Checkbox -->
        <div
          class="flex items-center gap-2 mt-1 cursor-pointer"
          onclick={() => (recursive = !recursive)}
          role="button"
          tabindex="0"
          onkeydown={(e) => e.key === 'Enter' && (recursive = !recursive)}
        >
          <div
            class="w-4 h-4 rounded border flex items-center justify-center transition-colors {recursive
              ? 'bg-nordic-cyan border-nordic-cyan text-[#041017]'
              : 'bg-[#0f172a] border-[#334155]'}"
          >
            {#if recursive}
              <IconCheck size={12} />
            {/if}
          </div>
          <span class="text-slate-300 text-[12px]">Scan subdirectories recursively</span>
        </div>

        <!-- Footer -->
        <div
          class="mt-3 -mx-4 -mb-4 px-4 py-3 bg-[#090e17] border-t border-nordic-border flex justify-end gap-2.5"
        >
          <button
            type="button"
            onclick={onClose}
            disabled={saving}
            class="bg-[#111723] hover:bg-[#162030] border border-nordic-border text-slate-300 px-3.5 py-1.5 rounded-md text-xs font-semibold transition"
          >
            Cancel
          </button>
          <button
            type="submit"
            disabled={saving}
            class="bg-nordic-cyan hover:bg-[#22d3ee] text-[#041017] px-4 py-1.5 rounded-md text-xs font-bold shadow-glow transition disabled:opacity-50"
          >
            {saving ? 'Saving...' : 'Save & Scan Now'}
          </button>
        </div>
      </form>
    </div>
  </div>
{/if}
