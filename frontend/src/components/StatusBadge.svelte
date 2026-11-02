<script lang="ts">
  import { statusLabel, statusTone } from '../lib/format';

  let { status }: { status: number } = $props();
</script>

<span class="badge tone-{statusTone(status)}">
  <span class="dot" aria-hidden="true"></span>
  {statusLabel(status)}
</span>

<style>
  .badge {
    display: inline-flex;
    align-items: center;
    gap: 0.45rem;
    font-family: var(--font-mono);
    font-size: 0.6875rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    white-space: nowrap;
  }

  .dot {
    width: 0.4rem;
    height: 0.4rem;
    border-radius: 50%;
    background: currentColor;
  }
</style>
