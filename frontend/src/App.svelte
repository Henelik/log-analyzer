<script lang="ts">
  import DetectionList from './components/DetectionList.svelte';
  import ErrorList from './components/ErrorList.svelte';
  import Login from './components/Login.svelte';
  import StatusBadge from './components/StatusBadge.svelte';
  import UploadPanel from './components/UploadPanel.svelte';
  import { untrack } from 'svelte';
  import {
    ApiError,
    clearSession,
    getSession,
    listFiles,
    type FileRecord,
    type Session,
    type UploadResult
  } from './lib/api';
  import { formatDate, formatNumber, statusNote } from './lib/format';

  let session = $state<Session | null>(getSession());
  let files = $state<FileRecord[]>([]);
  let selectedId = $state<number | null>(null);
  let loading = $state(false);
  let error = $state('');
  let tab = $state<'detections' | 'errors'>('detections');

  const selected = $derived(files.find((file) => file.id === selectedId) ?? null);

  async function refresh(preferredId?: number) {
    loading = true;
    error = '';

    try {
      const result = await listFiles();
      files = result;

      const wanted = preferredId ?? selectedId;
      if (wanted !== null && result.some((file) => file.id === wanted)) {
        selectedId = wanted;
      } else {
        selectedId = result.length > 0 ? result[0].id : null;
      }
    } catch (cause) {
      if (cause instanceof ApiError && cause.status === 401) {
        signOut();
        return;
      }

      error = cause instanceof Error ? cause.message : 'Could not load files.';
    } finally {
      loading = false;
    }
  }

  $effect(() => {
    if (session === null) {
      return;
    }

    untrack(() => void refresh());

    const timer = setInterval(() => {
      const pending = untrack(() => files.some((file) => file.status < 3));
      if (pending) {
        untrack(() => void refresh());
      }
    }, 10_000);

    return () => clearInterval(timer);
  });

  function signOut() {
    clearSession();
    session = null;
    files = [];
    selectedId = null;
  }

  function onUploaded(file: UploadResult) {
    tab = 'detections';
    void refresh(file.id);
  }

  function choose(id: number) {
    selectedId = id;
    tab = 'detections';
  }
</script>

{#if session === null}
  <Login onLogin={(next) => (session = next)} />
{:else}
  <div class="console">
    <header class="topbar">
      <div class="brand">
        <span class="mark mono">LA</span>
        <span class="name">Log Analyzer</span>
      </div>

      <div class="meta">
        <span class="mono user">{session.username}</span>
        <button class="button button--ghost small" onclick={signOut}>Sign out</button>
      </div>
    </header>

    <div class="layout">
      <aside class="rail" aria-labelledby="files-heading">
        <div class="rail-head">
          <div>
            <span class="kicker">ingests</span>
            <h2 id="files-heading">Files</h2>
          </div>
          <button
            class="refresh mono"
            onclick={() => void refresh()}
            disabled={loading}
            aria-label="Refresh file list"
          >
            ↻
          </button>
        </div>

        {#if loading && files.length === 0}
          <p class="note">Loading…</p>
        {:else if error}
          <p class="note error" role="alert">{error}</p>
        {:else if files.length === 0}
          <p class="note">No uploads yet. Start on the right.</p>
        {:else}
          <ul class="file-list">
            {#each files as file (file.id)}
              <li>
                <button
                  class="file"
                  class:active={file.id === selectedId}
                  onclick={() => choose(file.id)}
                  aria-current={file.id === selectedId}
                >
                  <span class="file-name">{file.filename}</span>
                  <span class="file-meta">
                    <StatusBadge status={file.status} />
                    <span class="mono id">#{file.id}</span>
                  </span>
                </button>
              </li>
            {/each}
          </ul>
        {/if}
      </aside>

      <main class="stage">
        <UploadPanel onUploaded={onUploaded} />

        {#if selected === null}
          <div class="placeholder">
            <span class="kicker">no selection</span>
            <p>Select an ingest to inspect detections and rejected rows.</p>
          </div>
        {:else}
          <article class="detail" aria-labelledby="detail-heading">
            <div class="detail-head">
              <div class="title-block">
                <span class="kicker">ingest #{selected.id}</span>
                <h1 id="detail-heading">{selected.filename}</h1>
                <p class="note">{statusNote(selected.status)}</p>
              </div>
              <StatusBadge status={selected.status} />
            </div>

            <dl class="stats">
              <div>
                <dt>Lines</dt>
                <dd class="mono">{formatNumber(selected.lines)}</dd>
              </div>
              <div>
                <dt>Accepted</dt>
                <dd class="mono accept">{formatNumber(selected.accepted_lines)}</dd>
              </div>
              <div>
                <dt>Rejected</dt>
                <dd class="mono reject">{formatNumber(selected.rejected_lines)}</dd>
              </div>
              <div>
                <dt>Duplicates</dt>
                <dd class="mono">{formatNumber(selected.duplicate_lines)}</dd>
              </div>
              <div>
                <dt>Uploaded</dt>
                <dd>{formatDate(selected.created_at)}</dd>
              </div>
            </dl>

            <div class="tabs" role="tablist" aria-label="Ingest detail">
              <button
                role="tab"
                id="tab-detections"
                aria-selected={tab === 'detections'}
                aria-controls="panel-detections"
                class="tab"
                class:active={tab === 'detections'}
                onclick={() => (tab = 'detections')}
              >
                Detections
              </button>
              <button
                role="tab"
                id="tab-errors"
                aria-selected={tab === 'errors'}
                aria-controls="panel-errors"
                class="tab"
                class:active={tab === 'errors'}
                onclick={() => (tab = 'errors')}
              >
                Rejected rows
              </button>
            </div>

            <div
              id="panel-detections"
              role="tabpanel"
              aria-labelledby="tab-detections"
              hidden={tab !== 'detections'}
            >
              {#if tab === 'detections'}
                {#key selected.status}
                  <DetectionList fileId={selected.id} status={selected.status} />
                {/key}
              {/if}
            </div>

            <div
              id="panel-errors"
              role="tabpanel"
              aria-labelledby="tab-errors"
              hidden={tab !== 'errors'}
            >
              {#if tab === 'errors'}
                {#key selected.status}
                  <ErrorList
                    fileId={selected.id}
                    status={selected.status}
                    headers={selected.headers}
                  />
                {/key}
              {/if}
            </div>
          </article>
        {/if}
      </main>
    </div>
  </div>
{/if}

<style>
  .console {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
  }

  .topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 1rem;
    padding: 0.85rem clamp(1rem, 3vw, 2rem);
    border-bottom: 1px solid var(--rule);
    background: var(--paper);
    position: sticky;
    top: 0;
    z-index: 10;
  }

  .brand {
    display: flex;
    align-items: center;
    gap: 0.65rem;
  }

  .mark {
    display: grid;
    place-items: center;
    width: 1.75rem;
    height: 1.75rem;
    background: var(--signal);
    color: var(--paper);
    font-size: 0.6875rem;
    font-weight: 700;
    letter-spacing: 0.04em;
  }

  .name {
    font-size: 0.9375rem;
    font-weight: 650;
    letter-spacing: -0.01em;
  }

  .meta {
    display: flex;
    align-items: center;
    gap: 1rem;
  }

  .user {
    font-size: 0.6875rem;
    letter-spacing: 0.06em;
    color: var(--ink-faint);
  }

  .small {
    padding: 0.4rem 0.7rem;
    font-size: 0.625rem;
  }

  .layout {
    display: grid;
    grid-template-columns: minmax(15rem, 19rem) minmax(0, 1fr);
    flex: 1;
    align-items: start;
  }

  .rail {
    display: flex;
    flex-direction: column;
    gap: 1rem;
    padding: 1.5rem clamp(1rem, 2vw, 1.5rem);
    border-right: 1px solid var(--rule);
    position: sticky;
    top: 3.5rem;
    max-height: calc(100vh - 3.5rem);
    overflow-y: auto;
  }

  .rail-head {
    display: flex;
    align-items: end;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .rail-head > div {
    display: flex;
    flex-direction: column;
    gap: 0.3rem;
  }

  h2 {
    font-size: 1.125rem;
    font-weight: 650;
    letter-spacing: -0.01em;
  }

  .refresh {
    padding: 0.2rem 0.4rem;
    font-size: 1rem;
    color: var(--ink-faint);
    transition: color 140ms ease;
  }

  .refresh:hover:not(:disabled) {
    color: var(--signal);
  }

  .refresh:disabled {
    opacity: 0.4;
    cursor: wait;
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

  .file-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
  }

  .file {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    width: 100%;
    padding: 0.7rem 0.6rem;
    text-align: left;
    border-left: 2px solid transparent;
    transition:
      background-color 140ms ease,
      border-color 140ms ease;
  }

  .file:hover {
    background: var(--paper-deep);
  }

  .file.active {
    border-left-color: var(--signal);
    background: var(--paper-deep);
  }

  .file-name {
    font-size: 0.875rem;
    font-weight: 550;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .file-meta {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem;
  }

  .id {
    font-size: 0.625rem;
    color: var(--ink-faint);
  }

  .stage {
    display: flex;
    flex-direction: column;
    gap: clamp(1.5rem, 3vw, 2.25rem);
    padding: clamp(1.25rem, 3vw, 2.25rem);
    min-width: 0;
  }

  .placeholder {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    padding: 3rem 0;
    color: var(--ink-faint);
  }

  .detail {
    display: flex;
    flex-direction: column;
    gap: 1.75rem;
    animation: enter 320ms cubic-bezier(0.16, 1, 0.3, 1) both;
  }

  .detail-head {
    display: flex;
    align-items: start;
    justify-content: space-between;
    gap: 1.5rem;
    padding-bottom: 1.25rem;
    border-bottom: 1px solid var(--rule-strong);
  }

  .title-block {
    display: flex;
    flex-direction: column;
    gap: 0.4rem;
    min-width: 0;
  }

  h1 {
    font-size: clamp(1.5rem, 3vw, 2.125rem);
    font-weight: 650;
    letter-spacing: -0.03em;
    overflow-wrap: anywhere;
  }

  .stats {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(6rem, 1fr));
    gap: 1rem;
    margin: 0;
    padding: 0 0 1.25rem;
    border-bottom: 1px solid var(--rule);
  }

  .stats div {
    display: flex;
    flex-direction: column;
    gap: 0.25rem;
  }

  .stats dt {
    font-family: var(--font-mono);
    font-size: 0.5625rem;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--ink-faint);
  }

  .stats dd {
    margin: 0;
    font-size: 1.125rem;
    font-variant-numeric: tabular-nums;
  }

  .stats dd.accept {
    color: var(--ok);
  }

  .stats dd.reject {
    color: var(--danger);
  }

  .tabs {
    display: flex;
    gap: 1.5rem;
    border-bottom: 1px solid var(--rule);
    margin-bottom: -0.75rem;
  }

  .tab {
    padding: 0 0 0.6rem;
    border-bottom: 2px solid transparent;
    font-family: var(--font-mono);
    font-size: 0.6875rem;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--ink-faint);
    transition:
      color 140ms ease,
      border-color 140ms ease;
  }

  .tab:hover {
    color: var(--ink);
  }

  .tab.active {
    color: var(--ink);
    border-bottom-color: var(--signal);
  }

  [role='tabpanel'] {
    animation: enter 240ms cubic-bezier(0.16, 1, 0.3, 1) both;
  }

  [hidden] {
    display: none;
  }

  @keyframes enter {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  @media (max-width: 780px) {
    .layout {
      grid-template-columns: 1fr;
    }

    .rail {
      position: static;
      max-height: none;
      border-right: 0;
      border-bottom: 1px solid var(--rule);
    }

    .file-list {
      flex-direction: row;
      overflow-x: auto;
    }

    .file-list li {
      flex: 0 0 13rem;
    }

    .detail-head {
      flex-direction: column;
      gap: 0.75rem;
    }
  }
</style>
