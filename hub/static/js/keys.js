// API key management on the account page.
// Renders per-entitlement key state and handles generate/reveal/revoke.
(async function () {
    const rows = document.querySelectorAll('tr[data-service]');
    if (!rows.length) return;

    // load current key metadata from the page data (server-rendered) — we re-derive per row via API state
    for (const row of rows) {
        const svc = row.getAttribute('data-service');
        const stateEl = row.querySelector('.key-state');
        const createdEl = row.querySelector('.key-created');
        const usedEl = row.querySelector('.key-used');
        try {
            const r = await fetch('/api/keys/' + svc + '/state');
            const d = await r.json();
            if (d.active) {
                stateEl.textContent = 'active';
                stateEl.className = 'key-state status-active';
                createdEl.textContent = d.created_at || '—';
                usedEl.textContent = d.last_used_at || 'never';
            } else {
                stateEl.textContent = 'none';
                stateEl.className = 'key-state status-none';
                createdEl.textContent = '—';
                usedEl.textContent = '—';
            }
        } catch (e) {
            stateEl.textContent = '—';
        }
    }

    document.addEventListener('click', async (ev) => {
        const btn = ev.target.closest('.key-btn');
        if (!btn) return;
        const row = btn.closest('tr[data-service]');
        const svc = row.getAttribute('data-service');
        const action = btn.getAttribute('data-action');
        if (action === 'generate') {
            if (!confirm('Generate a new API key for ' + svc + '? Any previous key stops working immediately.')) return;
            const r = await fetch('/api/keys/' + svc + '/generate', { method: 'POST' });
            const d = await r.json();
            if (!r.ok) { alert(d.error || 'Key generation failed'); return; }
            const cell = row.querySelector('.key-state');
            cell.innerHTML = '';
            const input = document.createElement('input');
            input.type = 'text'; input.value = d.key; input.readOnly = true;
            input.style.width = '100%'; input.style.fontSize = '0.8rem'; input.style.fontFamily = 'monospace';
            cell.appendChild(input);
            input.select();
            try { document.execCommand('copy'); } catch (e) { /* select is enough */ }
            const note = document.createElement('div');
            note.className = 'muted-note';
            note.textContent = 'Copied — shown once only';
            cell.appendChild(note);
            btn.textContent = 'Revoke';
            btn.setAttribute('data-action', 'revoke');
        } else if (action === 'revoke') {
            if (!confirm('Revoke the API key for ' + svc + '?')) return;
            const r = await fetch('/api/keys/' + svc + '/revoke', { method: 'POST' });
            if (r.ok) {
                const cell = row.querySelector('.key-state');
                cell.textContent = 'none';
                cell.className = 'key-state status-none';
                btn.textContent = 'Generate key';
                btn.setAttribute('data-action', 'generate');
            } else {
                const d = await r.json();
                alert(d.error || 'Revoke failed');
            }
        }
    });
})();
