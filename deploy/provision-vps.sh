#!/usr/bin/env bash
# katsironni VPS post-recreate provisioning — runs ON the VPS as root over SSH.
# Prereq: root key auth already verified working (key installed before this script).
set -euo pipefail

PUBKEY_THOTH="ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIJnzoDKdqw2LiU0qiPPxEBwRGCpLyIcycqMoaW1z2g6+ manitouabe8ich@Thoth"

echo "== [1/6] SSH hardening =="
mkdir -p /root/.ssh && chmod 700 /root/.ssh
touch /root/.ssh/authorized_keys
grep -qxF "$PUBKEY_THOTH" /root/.ssh/authorized_keys || echo "$PUBKEY_THOTH" >> /root/.ssh/authorized_keys
chmod 600 /root/.ssh/authorized_keys
mkdir -p /etc/ssh/sshd_config.d
cat > /etc/ssh/sshd_config.d/99-hardening.conf <<'EOF'
PermitRootLogin prohibit-password
PasswordAuthentication no
KbdInteractiveAuthentication no
PubkeyAuthentication yes
X11Forwarding no
AllowAgentForwarding no
MaxAuthTries 3
LoginGraceTime 30
EOF
sshd -t
systemctl reload ssh

echo "== [2/6] System packages =="
export DEBIAN_FRONTEND=noninteractive
apt-get update -y
apt-get -y upgrade
apt-get install -y ufw fail2ban unattended-upgrades sqlite3 git curl ca-certificates

echo "== [2b/6] Caddy (official repo) =="
apt-get install -y debian-keyring debian-archive-keyring apt-transport-https
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | gpg --batch --yes --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' > /etc/apt/sources.list.d/caddy-stable.list
apt-get update -y
apt-get install -y caddy

echo "== [3/6] Firewall =="
ufw default deny incoming
ufw default allow outgoing
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
yes | ufw enable

echo "== [4/6] fail2ban =="
mkdir -p /etc/fail2ban/jail.d
cat > /etc/fail2ban/jail.d/sshd.local <<'EOF'
[sshd]
enabled = true
maxretry = 4
bantime = 1h
EOF
systemctl enable --now fail2ban
systemctl restart fail2ban

echo "== [5/6] Go 1.27.2 =="
curl -fsSL "https://go.dev/dl/go1.27.2.linux-amd64.tar.gz" -o /tmp/go.tgz
tar -C /usr/local -xzf /tmp/go.tgz && rm -f /tmp/go.tgz
echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh

echo "== [6/6] App scaffold =="
id -u katsironni >/dev/null 2>&1 || useradd --system --home-dir /opt/katsironni --shell /usr/sbin/nologin katsironni
mkdir -p /opt/katsironni/{bin,data,services,web}
mkdir -p /opt/katsironni/services/{proofspan,switchyard,warmline,oneprobe}/data
touch /opt/katsironni/.env
chmod 750 /opt/katsironni
chmod 600 /opt/katsironni/.env
chown -R katsironni:katsironni /opt/katsironni

systemctl enable --now unattended-upgrades || true

echo "PROVISION COMPLETE"
