// Per-service agent chat widget. Connects to the hub's WebSocket endpoint,
// renders markdown replies (marked.js), supports file attachments and quick
// actions, and enforces the client side of the free-preview limit.
(function () {
    const shell = document.querySelector('.chat-shell');
    if (!shell) return;
    const service = shell.getAttribute('data-service');
    const log = document.getElementById('chatLog');
    const statusEl = document.getElementById('chatStatus');
    const form = document.getElementById('chatForm');
    const textEl = document.getElementById('chatText');
    const fileInput = document.getElementById('chatFile');
    const fileBtn = document.getElementById('chatFileBtn');
    const filesBox = document.getElementById('chatFiles');
    const dropZone = document.getElementById('chatDrop');
    const subPrompt = document.getElementById('chatSubPrompt');
    const quickBox = document.getElementById('chatQuick');

    let ws = null;
    let pendingFiles = [];   // [{id, name}]
    let remainingPreview = null;
    let subscribed = false;
    let reconnectTimer = null;

    function proto() { return location.protocol === 'https:' ? 'wss:' : 'ws:'; }
    function connect() {
        ws = new WebSocket(proto() + '//' + location.host + '/ws/chat/' + service);
        statusEl.textContent = 'connecting…';
        ws.onopen = () => { statusEl.textContent = 'online'; };
        ws.onclose = () => {
            statusEl.textContent = 'offline — reconnecting';
            clearTimeout(reconnectTimer);
            reconnectTimer = setTimeout(connect, 3000);
        };
        ws.onerror = () => { try { ws.close(); } catch (e) {} };
        ws.onmessage = (ev) => {
            let msg;
            try { msg = JSON.parse(ev.data); } catch (e) { return; }
            if (msg.type === 'agent') {
                removeTyping();
                addBubble('agent', msg.text);
                if (typeof msg.remaining_preview === 'number') {
                    remainingPreview = msg.remaining_preview;
                    updatePreviewUI();
                }
            } else if (msg.type === 'usage') {
                subscribed = msg.subscribed;
                if (typeof msg.remaining_preview === 'number') {
                    remainingPreview = msg.remaining_preview;
                    updatePreviewUI();
                }
            } else if (msg.type === 'error') {
                removeTyping();
                if (msg.subscribe_prompt) {
                    addBubble('agent', '🎯 Free preview complete — subscribe to continue. Your earlier messages are saved with your account.');
                    subPrompt.classList.remove('hidden');
                } else {
                    addBubble('agent', '⚠️ ' + (msg.error || 'unknown error'));
                }
            }
        };
    }

    function updatePreviewUI() {
        if (subscribed || remainingPreview === null) return;
        statusEl.textContent = 'online · ' + remainingPreview + ' free message' + (remainingPreview === 1 ? '' : 's') + ' left';
        if (remainingPreview <= 0) {
            subPrompt.classList.remove('hidden');
            textEl.disabled = true;
            document.getElementById('chatSend').disabled = true;
        }
    }

    function addBubble(role, text) {
        const empty = log.querySelector('.chat-empty');
        if (empty) empty.remove();
        const el = document.createElement('div');
        el.className = 'chat-msg chat-msg-' + role;
        const body = document.createElement('div');
        body.className = 'chat-msg-body';
        if (role === 'agent' && window.marked) {
            body.innerHTML = marked.parse(text, { breaks: true });
        } else {
            body.textContent = text;
        }
        el.appendChild(body);
        log.appendChild(el);
        log.scrollTop = log.scrollHeight;
    }

    function addTyping() {
        const el = document.createElement('div');
        el.className = 'chat-msg chat-msg-agent chat-typing';
        el.innerHTML = '<div class="chat-msg-body"><span class="dot"></span><span class="dot"></span><span class="dot"></span></div>';
        log.appendChild(el);
        log.scrollTop = log.scrollHeight;
        return el;
    }
    function removeTyping() { const t = log.querySelector('.chat-typing'); if (t) t.remove(); }

    function send(text) {
        if (!ws || ws.readyState !== 1) { addBubble('agent', 'Reconnecting… try again in a moment.'); connect(); return; }
        addBubble('user', text);
        addTyping();
        ws.send(JSON.stringify({
            type: 'message',
            text: text,
            files: pendingFiles.map(f => f.id),
        }));
        pendingFiles = [];
        renderFiles();
    }

    form.addEventListener('submit', (ev) => {
        ev.preventDefault();
        const text = textEl.value.trim();
        if (!text) return;
        textEl.value = '';
        send(text);
    });
    textEl.addEventListener('keydown', (ev) => {
        if (ev.key === 'Enter' && !ev.shiftKey) { ev.preventDefault(); form.requestSubmit(); }
    });
    if (quickBox) {
        quickBox.addEventListener('click', (ev) => {
            const btn = ev.target.closest('.chat-quick-btn');
            if (btn) send(btn.textContent.trim());
        });
    }

    // attachments
    function renderFiles() {
        filesBox.innerHTML = '';
        pendingFiles.forEach((f, i) => {
            const chip = document.createElement('span');
            chip.className = 'file-chip';
            chip.textContent = f.name;
            const x = document.createElement('a');
            x.textContent = '×';
            x.href = '#';
            x.onclick = (e) => { e.preventDefault(); pendingFiles.splice(i, 1); renderFiles(); };
            chip.appendChild(x);
            filesBox.appendChild(chip);
        });
    }
    async function uploadFiles(fileList) {
        for (const file of fileList) {
            if (file.size > 10 * 1024 * 1024) { alert(file.name + ': over 10MB'); continue; }
            const fd = new FormData();
            fd.append('file', file);
            try {
                const r = await fetch('/api/chat/' + service + '/upload', { method: 'POST', body: fd });
                const d = await r.json();
                if (r.ok) { pendingFiles.push({ id: d.id, name: d.name }); renderFiles(); }
                else alert(d.error || 'upload failed');
            } catch (e) { alert('upload failed'); }
        }
    }
    fileBtn.addEventListener('click', () => fileInput.click());
    fileInput.addEventListener('change', () => { uploadFiles([...fileInput.files]); fileInput.value = ''; });
    ['dragenter', 'dragover'].forEach(t => dropZone.addEventListener(t, e => { e.preventDefault(); dropZone.classList.add('drag'); }));
    ['dragleave', 'drop'].forEach(t => dropZone.addEventListener(t, e => { e.preventDefault(); dropZone.classList.remove('drag'); }));
    dropZone.addEventListener('drop', e => { if (e.dataTransfer.files.length) uploadFiles([...e.dataTransfer.files]); });

    connect();
})();
