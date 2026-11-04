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
  <!-- Handle -->
  <path d="M18 3L9.5 11.5" />
  <!-- Ferrule collar -->
  <path d="M7 14L10 17" stroke-width="2.5" />
  <!-- Bristle head fan -->
  <path
    d="M7 14C5 15.5 3 18 3 20C3 20.6 3.4 21 4 21C6 21 8.5 19 10 17Z"
    fill="currentColor"
    fill-opacity="0.2"
  />
  <!-- Bristle details -->
  <path d="M5.5 17.5L4 20" stroke-width="1.5" />
  <path d="M7.5 18.5L6.5 20.5" stroke-width="1.5" />
  <!-- Clean spark stars & particles -->
  <path d="M4 4L4.5 5.5L6 6L4.5 6.5L4 8L3.5 6.5L2 6L3.5 5.5Z" fill="currentColor" stroke="none" />
  <circle cx="19" cy="15" r="1.5" fill="currentColor" stroke="none" />
  <circle cx="16" cy="19" r="1" fill="currentColor" stroke="none" />
</svg>
