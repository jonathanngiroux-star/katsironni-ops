// MetaMask SIWE login for katsironni.com
// Flow: connect wallet -> server nonce+message -> personal_sign -> server verifies -> session
async function siweLogin() {
    const btn = document.querySelector('.btn-metamask, .btn-social');
    if (!window.ethereum) {
        alert('No Ethereum wallet detected. Install MetaMask, or use email sign-in.');
        return;
    }
    try {
        const accounts = await window.ethereum.request({ method: 'eth_requestAccounts' });
        const address = accounts[0];

        const nres = await fetch('/auth/siwe/nonce', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ address: address })
        });
        const ndata = await nres.json();
        if (!nres.ok || !ndata.message) {
            alert(ndata.error || 'Could not start wallet sign-in.');
            return;
        }

        const signature = await window.ethereum.request({
            method: 'personal_sign',
            params: [ndata.message, address]
        });

        const vres = await fetch('/auth/siwe/verify', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ address: address, message: ndata.message, signature: signature })
        });
        const vdata = await vres.json();
        if (vres.ok && vdata.ok) {
            window.location.href = '/account';
        } else {
            alert(vdata.error || 'Wallet signature verification failed.');
        }
    } catch (err) {
        if (err && err.code === 4001) return; // user rejected the signature — stay put
        console.error('siwe error', err);
        alert('Wallet sign-in failed. Please try again or use email.');
    }
}
