<script lang="ts">
  import { login, saveSession, type Session } from '../lib/api';

  let { onLogin }: { onLogin: (session: Session) => void } = $props();

  let username = $state('');
  let password = $state('');
  let submitting = $state(false);
  let error = $state('');

  async function submit(event: SubmitEvent) {
    event.preventDefault();

    if (username === '' || password === '') {
      error = 'Enter username and password.';
      return;
    }

    submitting = true;
    error = '';

    try {
      const token = await login(username, password);
      const session: Session = { token, username };
      saveSession(session);
      onLogin(session);
    } catch (cause) {
      error = cause instanceof Error ? cause.message : 'Login failed.';
    } finally {
      submitting = false;
    }
  }
</script>

<main class="shell">
  <section class="masthead" aria-labelledby="brand">
    <span class="kicker">internal security console</span>
    <h1 id="brand">Log<br /><span>Analyzer</span></h1>
    <p class="lede">
      Ingest CSV event logs. The parser flags malformed rows. Detectors surface brute force,
      port scanning, and exfiltration.
    </p>

    <dl class="facts">
      <div>
        <dt>Ingest</dt>
        <dd>CSV, streaming upload</dd>
      </div>
      <div>
        <dt>Parse</dt>
        <dd>Out-of-band worker</dd>
      </div>
      <div>
        <dt>Detect</dt>
        <dd>IOC analysis per file</dd>
      </div>
    </dl>

    <span class="index" aria-hidden="true">01 / 03</span>
  </section>

  <section class="panel" aria-labelledby="login-heading">
    <div class="panel-head">
      <span class="kicker">authentication</span>
      <h2 id="login-heading">Sign in</h2>
    </div>

    <form onsubmit={submit} novalidate>
      <label for="username">Username</label>
      <input
        id="username"
        name="username"
        autocomplete="username"
        bind:value={username}
        disabled={submitting}
      />

      <label for="password">Password</label>
      <input
        id="password"
        name="password"
        type="password"
        autocomplete="current-password"
        bind:value={password}
        disabled={submitting}
      />

      {#if error}
        <p class="error" role="alert">{error}</p>
      {/if}

      <button class="button" type="submit" disabled={submitting}>
        {submitting ? 'Verifying' : 'Enter console'}
      </button>
    </form>

    <p class="footnote">
      Sessions are held in the browser after the API issues a signed token.
    </p>
  </section>
</main>

<style>
  .shell {
    display: grid;
    grid-template-columns: minmax(0, 1.15fr) minmax(20rem, 0.85fr);
    min-height: 100vh;
  }

  .masthead {
    display: flex;
    flex-direction: column;
    gap: 2rem;
    padding: clamp(2rem, 5vw, 4.5rem);
    border-right: 1px solid var(--rule);
    position: relative;
    animation: rise 520ms cubic-bezier(0.16, 1, 0.3, 1) both;
  }

  h1 {
    font-size: clamp(3.25rem, 9vw, 6.5rem);
    font-weight: 700;
    line-height: 0.86;
    letter-spacing: -0.045em;
  }

  h1 span {
    color: var(--signal);
  }

  .lede {
    max-width: 30rem;
    font-size: 1.0625rem;
    line-height: 1.6;
    color: var(--ink-soft);
  }

  .facts {
    display: grid;
    gap: 0;
    margin: 0;
    max-width: 32rem;
    border-top: 1px solid var(--rule);
  }

  .facts div {
    display: grid;
    grid-template-columns: 7rem 1fr;
    gap: 1rem;
    padding: 0.85rem 0;
    border-bottom: 1px solid var(--rule);
  }

  .facts dt {
    font-family: var(--font-mono);
    font-size: 0.6875rem;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--ink-faint);
    padding-top: 0.15rem;
  }

  .facts dd {
    margin: 0;
    font-size: 0.9375rem;
  }

  .index {
    margin-top: auto;
    font-family: var(--font-mono);
    font-size: 0.6875rem;
    letter-spacing: 0.18em;
    color: var(--ink-faint);
  }

  .panel {
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 2rem;
    padding: clamp(2rem, 5vw, 4rem);
    background: var(--paper-deep);
    animation: rise 520ms 90ms cubic-bezier(0.16, 1, 0.3, 1) both;
  }

  .panel-head {
    display: flex;
    flex-direction: column;
    gap: 0.6rem;
  }

  h2 {
    font-size: 1.75rem;
    font-weight: 650;
    letter-spacing: -0.02em;
  }

  form {
    display: flex;
    flex-direction: column;
    gap: 0.5rem;
  }

  label {
    margin-top: 0.85rem;
    font-family: var(--font-mono);
    font-size: 0.6875rem;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--ink-faint);
  }

  input {
    width: 100%;
    padding: 0.7rem 0;
    border: 0;
    border-bottom: 1px solid var(--rule-strong);
    background: transparent;
    font-size: 1.0625rem;
    transition: border-color 140ms ease;
  }

  input:focus {
    outline: none;
    border-bottom-color: var(--signal);
  }

  input:disabled {
    opacity: 0.55;
  }

  .button {
    align-self: flex-start;
    margin-top: 1.75rem;
  }

  .error {
    margin-top: 0.75rem;
    font-family: var(--font-mono);
    font-size: 0.75rem;
    color: var(--danger);
  }

  .footnote {
    font-size: 0.8125rem;
    line-height: 1.5;
    color: var(--ink-faint);
  }

  @keyframes rise {
    from {
      opacity: 0;
      transform: translateY(14px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  @media (max-width: 820px) {
    .shell {
      grid-template-columns: 1fr;
    }

    .masthead {
      border-right: 0;
      border-bottom: 1px solid var(--rule);
    }

    .index {
      display: none;
    }
  }
</style>
