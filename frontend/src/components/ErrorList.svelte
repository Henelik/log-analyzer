<script lang="ts">
  import { getEventErrors, type EventError } from '../lib/api';

  let { fileId, status }: { fileId: number; status: number } = $props();

  let errors = $state<EventError[]>([]);
  let loading = $state(true);
  let error = $state('');

  $effect(() => {
    const id = fileId;

    let active = true;
    loading = true;
    error = '';

    getEventErrors(id)
      .then((result) => {
        if (active) {
          errors = result;
        }
      })
      .catch((cause: unknown) => {
        if (active) {
          error = cause instanceof Error ? cause.message : 'Could not load parse errors.';
        }
      })
      .finally(() => {
        if (active) {
          loading = false;
        }
      });

    return () => {
      active = false;
    };
  });
</script>

<section class="block" aria-labelledby="errors-heading">
  <div class="block-head">
    <div>
      <span class="kicker">parse</span>
      <h3 id="errors-heading">Rejected rows</h3>
    </div>
    {#if !loading && !error}
      <span class="count mono">{errors.length} rows</span>
    {/if}
  </div>

  {#if loading}
    <p class="note">Loading parse errors…</p>
  {:else if error}
    <p class="note error" role="alert">{error}</p>
  {:else if errors.length === 0}
    <div class="empty">
      <p class="empty-title">
        {status < 2 ? 'Parsing pending' : 'Every row parsed cleanly'}
      </p>
      <p class="note">
        {status < 2
          ? 'The parser runs every 30 seconds. Invalid rows appear here after this ingest is parsed.'
          : 'Rows that fail validation appear here with their original content.'}
      </p>
    </div>
  {:else}
    <ul class="list">
      {#each errors as item (item.line_number)}
        <li class="item">
          <div class="row-head">
            <span class="line mono">line {item.line_number}</span>
            <span class="message">{item.error}</span>
          </div>
          <pre class="raw mono">{item.raw_log}</pre>
        </li>
      {/each}
    </ul>
  {/if}
</section>

<style>
  .block {
    display: flex;
    flex-direction: column;
    gap: 1rem;
  }

  .block-head {
    display: flex;
    align-items: end;
    justify-content: space-between;
    gap: 1rem;
    padding-bottom: 0.75rem;
    border-bottom: 1px solid var(--rule);
  }

  .block-head > div {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
  }

  h3 {
    font-size: 1.125rem;
    font-weight: 650;
    letter-spacing: -0.01em;
  }

  .count {
    font-size: 0.6875rem;
    letter-spacing: 0.08em;
    color: var(--ink-faint);
  }

  .note {
    font-size: 0.8125rem;
    line-height: 1.5;
    color: var(--ink-faint);
  }

  .error {
    color: var(--danger);
    font-family: var(--font-mono);
  }

  .empty {
    display: flex;
    flex-direction: column;
    gap: 0.35rem;
    padding: 1.25rem 0;
  }

  .empty-title {
    font-size: 0.9375rem;
    font-weight: 600;
    color: var(--ok);
  }

  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
  }

  .item {
    display: flex;
    flex-direction: column;
    gap: 0.45rem;
    padding: 0.9rem 0;
    border-bottom: 1px solid var(--rule);
  }

  .row-head {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    gap: 0.75rem;
  }

  .line {
    font-size: 0.6875rem;
    letter-spacing: 0.08em;
    text-transform: uppercase;
    color: var(--danger);
  }

  .message {
    font-size: 0.8125rem;
    color: var(--ink-soft);
  }

  .raw {
    margin: 0;
    padding: 0.6rem 0.75rem;
    background: var(--paper-deep);
    border-left: 2px solid var(--rule-strong);
    font-size: 0.75rem;
    line-height: 1.5;
    color: var(--ink-soft);
    white-space: pre-wrap;
    word-break: break-all;
  }
</style>
