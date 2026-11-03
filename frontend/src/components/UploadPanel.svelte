<script lang="ts">
  import { uploadFile, type UploadResult } from '../lib/api';

  let { onUploaded }: { onUploaded: (file: UploadResult) => void } = $props();

  let dragging = $state(false);
  let uploading = $state(false);
  let error = $state('');
  let pendingName = $state('');

  let input = $state<HTMLInputElement | null>(null);

  async function send(file: File | undefined) {
    if (file === undefined || uploading) {
      return;
    }

    uploading = true;
    error = '';
    pendingName = file.name;

    try {
      const record = await uploadFile(file);
      onUploaded(record);
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'Upload failed.';
    } finally {
      uploading = false;
      pendingName = '';

      if (input !== null) {
        input.value = '';
      }
    }
  }

  function onDrop(event: DragEvent) {
    event.preventDefault();
    dragging = false;
    void send(event.dataTransfer?.files?.[0]);
  }

  function onChange(event: Event) {
    const target = event.currentTarget as HTMLInputElement;
    void send(target.files?.[0]);
  }
</script>

<section
  class="dropzone"
  class:dragging
  class:uploading
  aria-labelledby="upload-heading"
  ondragover={(event) => {
    event.preventDefault();
    dragging = true;
  }}
  ondragleave={() => (dragging = false)}
  ondrop={onDrop}
>
  <div class="copy">
    <span class="kicker">ingest</span>
    <h2 id="upload-heading">Upload a log file</h2>
    <p>
      ZScaler format CSV with an <code class="mono">epochtime</code> column.
    </p>
  </div>

  <div class="actions">
    <input
      bind:this={input}
      id="log-file"
      class="visually-hidden"
      type="file"
      accept=".csv,text/csv"
      onchange={onChange}
      disabled={uploading}
    />
    <label class="button" for="log-file" aria-disabled={uploading}>
      {uploading ? 'Uploading' : 'Choose file'}
    </label>
    <span class="hint mono">or drop a .csv here</span>
  </div>

  {#if pendingName}
    <p class="status mono" aria-live="polite">{pendingName} → queue</p>
  {/if}

  {#if error}
    <p class="error" role="alert">{error}</p>
  {/if}
</section>

<style>
  .dropzone {
    display: flex;
    flex-wrap: wrap;
    align-items: end;
    justify-content: space-between;
    gap: 1.5rem;
    padding: 1.75rem clamp(1.25rem, 3vw, 2rem);
    border: 1px dashed var(--rule-strong);
    background: var(--paper);
    transition:
      border-color 160ms ease,
      background-color 160ms ease;
  }

  .dropzone.dragging {
    border-color: var(--signal);
    background: var(--signal-soft);
  }

  .copy {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
    max-width: 34rem;
  }

  h2 {
    font-size: 1.25rem;
    font-weight: 650;
    letter-spacing: -0.015em;
  }

  p {
    font-size: 0.875rem;
    line-height: 1.55;
    color: var(--ink-soft);
  }

  code {
    font-size: 0.8125rem;
    padding: 0.1rem 0.3rem;
    background: var(--paper-deep);
    border: 1px solid var(--rule);
  }

  .actions {
    display: flex;
    flex-direction: column;
    align-items: start;
    gap: 0.5rem;
  }

  .actions .button {
    border-style: solid;
  }

  .actions label[aria-disabled='true'] {
    opacity: 0.5;
    pointer-events: none;
  }

  .hint {
    font-size: 0.6875rem;
    letter-spacing: 0.06em;
    color: var(--ink-faint);
  }

  .status {
    flex-basis: 100%;
    font-size: 0.75rem;
    color: var(--ink-soft);
  }

  .error {
    flex-basis: 100%;
    font-family: var(--font-mono);
    font-size: 0.75rem;
    color: var(--danger);
  }

  .visually-hidden {
    position: absolute;
    width: 1px;
    height: 1px;
    overflow: hidden;
    clip-path: inset(50%);
    white-space: nowrap;
  }
</style>
