<script lang="ts">
  let { size = 14, class: className = '' }: { size?: number; class?: string } = $props();
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="2"
  stroke-linecap="round"
  stroke-linejoin="round"
  class={className}
>
  <!-- Terminal Prompt Chevron ('>') -->
  <path d="M5 6L11 12L5 18" stroke-width="2.5" />
  <!-- Vacuum Clean Beam Rays -->
  <line x1="12" y1="12" x2="20" y2="12" stroke-width="2" />
  <line x1="12.5" y1="9" x2="18.5" y2="7" stroke-width="1.5" opacity="0.75" />
  <line x1="12.5" y1="15" x2="18.5" y2="17" stroke-width="1.5" opacity="0.75" />
  <!-- Disintegrated Bits -->
  <rect x="19" y="6" width="1.5" height="1.5" rx="0.5" fill="currentColor" stroke="none" />
  <rect x="20" y="11.25" width="1.5" height="1.5" rx="0.5" fill="currentColor" stroke="none" />
  <rect x="19" y="16.5" width="1.5" height="1.5" rx="0.5" fill="currentColor" stroke="none" />
  <!-- Clean Starburst -->
  <path d="M10 3.5L10.5 4.5L11.5 5L10.5 5.5L10 6.5L9.5 5.5L8.5 5L9.5 4.5Z" fill="currentColor" stroke="none" />
</svg>
