<script lang="ts">
  let { size = 14, class: className = '' }: { size?: number; class?: string } = $props();
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 24 24"
  fill="none"
  stroke="currentColor"
  stroke-width="2.5"
  stroke-linecap="round"
  stroke-linejoin="round"
  class={className}
>
  <polyline points="20 6 9 17 4 12"></polyline>
</svg>
