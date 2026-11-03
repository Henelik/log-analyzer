<script lang="ts">
  import { getEventErrors, type EventError } from '../lib/api';

  let {
    fileId,
    status,
    headers
  }: { fileId: number; status: number; headers: string } = $props();

  const parsePending = $derived(status === 1);

  let errors = $state<EventError[]>([]);
  let loading = $state(true);
  let error = $state('');

  interface TabularRow extends EventError {
    values: string[];
  }

  // Parse a single CSV row, matching the rules encoding/csv applies on the
  // server.  Returns null for malformed rows so they fall back to raw output.
  function parseRow(row: string): string[] | null {
    if (row === '') {
      return null;
    }

    const values: string[] = [];
    let value = '';
    let state: 'start' | 'plain' | 'quoted' | 'quoted-end' = 'start';
    let index = 0;

    while (index < row.length) {
      const char = row[index];

      switch (state) {
        case 'start':
        case 'plain':
          if (char === ',') {
            values.push(value);
            value = '';
            state = 'start';
            index++;
          } else if (char === '"') {
            if (state === 'plain') {
              return null;
            }
            state = 'quoted';
            index++;
          } else {
            value += char;
            state = 'plain';
            index++;
          }
          break;
        case 'quoted':
          if (char === '"') {
            if (row[index + 1] === '"') {
              value += '"';
              index += 2;
            } else {
              state = 'quoted-end';
              index++;
            }
          } else {
            value += char;
            index++;
          }
          break;
        case 'quoted-end':
          if (char === ',') {
            values.push(value);
            value = '';
            state = 'start';
            index++;
          } else {
            return null;
          }
          break;
      }
    }

    if (state === 'quoted') {
      return null;
    }

    values.push(value);

    return values;
  }

  const classified = $derived.by(() => {
    const columns = parseRow(headers) ?? [];
    const tabular: TabularRow[] = [];
    const raw: EventError[] = [];

    for (const item of errors) {
      const values = columns.length > 0 ? parseRow(item.raw_log) : null;

      if (values !== null && values.length === columns.length) {
        tabular.push({ ...item, values });
      } else {
        raw.push(item);
      }
    }

    return { columns, tabular, raw };
  });

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
      <div class="heading">
        <h3 id="errors-heading">Rejected rows</h3>
        {#if parsePending}
          <span class="spinner" role="status" aria-label="Parsing in progress"></span>
        {/if}
      </div>
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
          ? 'Invalid rows appear here after this ingest is parsed.'
          : 'Rows that fail validation appear here with their original content.'}
      </p>
    </div>
  {:else}
    {#if classified.tabular.length > 0}
      <div class="table-wrap">
        <table>
          <thead>
            <tr>
              <th scope="col">line</th>
              {#each classified.columns as column, index (index)}
                <th scope="col">{column}</th>
              {/each}
              <th scope="col">error</th>
            </tr>
          </thead>
          <tbody>
            {#each classified.tabular as item (item.line_number)}
              <tr>
                <td class="mono line">{item.line_number}</td>
                {#each item.values as value, index (index)}
                  <td class="mono value">{value || '—'}</td>
                {/each}
                <td class="error-text">{item.error}</td>
              </tr>
            {/each}
          </tbody>
        </table>
      </div>
    {/if}

    {#if classified.raw.length > 0}
      {#if classified.tabular.length > 0}
        <p class="note">Rows that do not match the header layout:</p>
      {/if}

      <ul class="list">
        {#each classified.raw as item (item.line_number)}
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

  .heading {
    display: flex;
    align-items: center;
    gap: 0.6rem;
  }

  .spinner {
    width: 0.85rem;
    height: 0.85rem;
    border: 2px solid var(--rule);
    border-top-color: var(--signal);
    border-radius: 50%;
    animation: spin 700ms linear infinite;
  }

  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
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

  .table-wrap {
    overflow-x: auto;
    border: 1px solid var(--rule);
    background: var(--paper);
  }

  table {
    width: 100%;
    border-collapse: collapse;
    font-size: 0.75rem;
  }

  th {
    padding: 0.5rem 0.75rem;
    text-align: left;
    font-family: var(--font-mono);
    font-size: 0.5625rem;
    font-weight: 500;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--ink-faint);
    border-bottom: 1px solid var(--rule);
    white-space: nowrap;
  }

  td {
    padding: 0.45rem 0.75rem;
    border-bottom: 1px solid var(--rule);
    white-space: nowrap;
  }

  td.line {
    color: var(--danger);
  }

  td.value {
    max-width: 22rem;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  td.error-text {
    color: var(--danger);
    white-space: normal;
  }

  tbody tr:last-child td {
    border-bottom: 0;
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

  .row-head .line {
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
