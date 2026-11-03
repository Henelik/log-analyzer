<script lang="ts">
  import { getDetections, getEvents, type Detection, type EventRecord } from '../lib/api';
  import { formatDate, formatNumber, level } from '../lib/format';

  let { fileId, status }: { fileId: number; status: number } = $props();

  const analysisPending = $derived(status === 1 || status === 2);

  let detections = $state<Detection[]>([]);
  let loading = $state(true);
  let error = $state('');

  let open = $state<number | null>(null);
  let events = $state<EventRecord[]>([]);
  let eventsLoading = $state(false);
  let eventsError = $state('');

  const windowLabel = $derived.by(() => {
    const times = events
      .map((event) => (event.time === null ? Number.NaN : new Date(event.time).getTime()))
      .filter((time) => !Number.isNaN(time));

    if (times.length === 0) {
      return '—';
    }

    const start = new Date(Math.min(...times));
    const end = new Date(Math.max(...times));

    if (start.getTime() === end.getTime()) {
      return formatDate(start.toISOString());
    }

    return `${formatDate(start.toISOString())} – ${formatDate(end.toISOString())}`;
  });

  $effect(() => {
    const id = fileId;

    let active = true;
    loading = true;
    error = '';
    open = null;
    events = [];

    getDetections(id)
      .then((result) => {
        if (active) {
          detections = result;
        }
      })
      .catch((cause: unknown) => {
        if (active) {
          error = cause instanceof Error ? cause.message : 'Could not load detections.';
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

  async function reveal(detection: Detection) {
    if (open === detection.detection_id) {
      open = null;
      return;
    }

    open = detection.detection_id;
    events = [];
    eventsError = '';

    if (detection.events.length === 0) {
      return;
    }

    eventsLoading = true;

    try {
      events = await getEvents(fileId, detection.events);
    } catch (cause) {
      eventsError = cause instanceof Error ? cause.message : 'Could not load events.';
    } finally {
      eventsLoading = false;
    }
  }
</script>

<section class="block" aria-labelledby="detections-heading">
  <div class="block-head">
    <div>
      <span class="kicker">analysis</span>
      <div class="heading">
        <h3 id="detections-heading">Detections</h3>
        {#if analysisPending}
          <span class="spinner" role="status" aria-label="Analysis in progress"></span>
        {/if}
      </div>
    </div>
    {#if !loading && !error}
      <span class="count mono">{detections.length} found</span>
    {/if}
  </div>

  {#if loading}
    <p class="note">Loading detections…</p>
  {:else if error}
    <p class="note error" role="alert">{error}</p>
  {:else if detections.length === 0}
    <div class="empty">
      <p class="empty-title">
        {status < 3 ? 'Analysis pending' : 'No indicators detected'}
      </p>
      <p class="note">
        {status < 3
          ? 'Detections appear here once this ingest is processed.'
          : 'No matching brute force, port scan, or exfiltration patterns were found in this ingest.'}
      </p>
    </div>
  {:else}
    <ol class="list">
      {#each detections as detection (detection.detection_id)}
        {@const openHere = open === detection.detection_id}
        <li class="item" class:open={openHere}>
          <button class="summary" onclick={() => reveal(detection)} aria-expanded={openHere}>
            <span class="rank mono" data-level={level(detection.severity)}>
              {String(detection.detection_id).padStart(2, '0')}
            </span>

            <span class="label">
              <span class="title">{detection.description}</span>
              <span class="entity mono">{detection.entity || 'unattributed'}</span>
            </span>

            <span class="scores">
              <span class="score">
                <span class="score-name">sev</span>
                <span class="score-value mono">{detection.severity}</span>
              </span>
              <span class="score">
                <span class="score-name">conf</span>
                <span class="score-value mono">{detection.confidence}</span>
              </span>
              <span class="score">
                <span class="score-name">prio</span>
                <span class="score-value mono">{detection.priority}</span>
              </span>
            </span>

            <span class="chevron" aria-hidden="true">{openHere ? '−' : '+'}</span>
          </button>

          {#if openHere}
            <div class="detail">
              <dl class="metrics">
                <div>
                  <dt>Window</dt>
                  <dd class="mono">{eventsLoading ? '…' : windowLabel}</dd>
                </div>
                <div>
                  <dt>Observed</dt>
                  <dd class="mono">{formatNumber(detection.observed)}</dd>
                </div>
                <div>
                  <dt>Threshold</dt>
                  <dd class="mono">{formatNumber(detection.threshold)}</dd>
                </div>
                <div>
                  <dt>Lines</dt>
                  <dd class="mono">{detection.events.length}</dd>
                </div>
              </dl>

              {#if detection.notes}
                <p class="notes">{detection.notes}</p>
              {/if}

              {#if eventsLoading}
                <p class="note">Loading related events…</p>
              {:else if eventsError}
                <p class="note error" role="alert">{eventsError}</p>
              {:else if events.length === 0}
                <p class="note">No related events returned.</p>
              {:else}
                <div class="table-wrap">
                  <table>
                    <thead>
                      <tr>
                        <th scope="col">Line</th>
                        <th scope="col">Time</th>
                        <th scope="col">User</th>
                        <th scope="col">Client</th>
                        <th scope="col">Port</th>
                        <th scope="col">Method</th>
                        <th scope="col">URL</th>
                        <th scope="col">Code</th>
                      </tr>
                    </thead>
                    <tbody>
                      {#each events as event (event.line_number)}
                        <tr>
                          <td class="mono">{event.line_number}</td>
                          <td class="mono">
                            {event.time ? new Date(event.time).toLocaleString() : '—'}
                          </td>
                          <td>{event.user || '—'}</td>
                          <td class="mono">{event.client_ip || '—'}</td>
                          <td class="mono">{event.server_port ?? '—'}</td>
                          <td class="mono">{event.method || '—'}</td>
                          <td class="url">{event.url || '—'}</td>
                          <td class="mono">{event.response_code ?? '—'}</td>
                        </tr>
                      {/each}
                    </tbody>
                  </table>
                </div>
              {/if}
            </div>
          {/if}
        </li>
      {/each}
    </ol>
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
  }

  .list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
  }

  .item {
    border-bottom: 1px solid var(--rule);
  }

  .summary {
    display: grid;
    grid-template-columns: 2.5rem minmax(0, 1fr) auto 1.5rem;
    align-items: center;
    gap: 0.75rem;
    width: 100%;
    padding: 0.85rem 0.25rem;
    text-align: left;
  }

  .summary:hover .title {
    color: var(--signal);
  }

  .rank {
    font-size: 0.8125rem;
    font-weight: 600;
    color: var(--ink-faint);
  }

  .rank[data-level='high'] {
    color: var(--danger);
  }

  .rank[data-level='elevated'] {
    color: var(--signal);
  }

  .rank[data-level='moderate'] {
    color: var(--warn);
  }

  .label {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
    min-width: 0;
  }

  .title {
    font-size: 0.9375rem;
    font-weight: 600;
    transition: color 140ms ease;
  }

  .entity {
    font-size: 0.6875rem;
    color: var(--ink-faint);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .scores {
    display: flex;
    gap: 1rem;
  }

  .score {
    display: flex;
    flex-direction: column;
    align-items: end;
    gap: 0.1rem;
  }

  .score-name {
    font-family: var(--font-mono);
    font-size: 0.5625rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--ink-faint);
  }

  .score-value {
    font-size: 0.8125rem;
    font-variant-numeric: tabular-nums;
  }

  .chevron {
    font-family: var(--font-mono);
    font-size: 1rem;
    color: var(--ink-faint);
    text-align: center;
  }

  .detail {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    padding: 0.25rem 0.25rem 1.5rem 3.5rem;
    animation: unfold 220ms cubic-bezier(0.16, 1, 0.3, 1) both;
  }

  .metrics {
    display: flex;
    gap: 2rem;
    margin: 0;
  }

  .metrics div {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
  }

  .metrics dt {
    font-family: var(--font-mono);
    font-size: 0.5625rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--ink-faint);
  }

  .metrics dd {
    margin: 0;
    font-size: 0.9375rem;
    font-variant-numeric: tabular-nums;
  }

  .notes {
    font-size: 0.8125rem;
    line-height: 1.55;
    color: var(--ink-soft);
    max-width: 48rem;
  }

  .description {
    font-size: 0.875rem;
    line-height: 1.6;
    color: var(--ink);
    max-width: 48rem;
    padding-left: 0.75rem;
    border-left: 2px solid var(--rule);
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

  tbody tr:last-child td {
    border-bottom: 0;
  }

  td.url {
    max-width: 22rem;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  @keyframes unfold {
    from {
      opacity: 0;
      transform: translateY(-4px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  @media (max-width: 640px) {
    .summary {
      grid-template-columns: 1.75rem minmax(0, 1fr) 1.25rem;
    }

    .scores {
      display: none;
    }

    .detail {
      padding-left: 0.25rem;
    }
  }
</style>
