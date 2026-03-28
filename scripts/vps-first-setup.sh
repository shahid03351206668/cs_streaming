#!/usr/bin/env bash
# =============================================================================
# VPS First-Time Setup — run ONCE on the server before the first CI/CD deploy.
# Usage: ssh ubuntu@your-vps "bash -s" < scripts/vps-first-setup.sh
# =============================================================================
set -e

ADMIN_DIR="/home/ubuntu/tasksy-admin"
SERVER_DIR="/home/ubuntu/tasksy-server"

# ── Directories ──────────────────────────────────────────────────────────────
mkdir -p "$ADMIN_DIR"
mkdir -p "$SERVER_DIR"
echo "[ok] Directories created"

# ── Node.js 20 (via NodeSource) ───────────────────────────────────────────────
if ! command -v node &>/dev/null; then
  echo "[..] Installing Node.js 20..."
  curl -fsSL https://deb.nodesource.com/setup_20.x | sudo -E bash -
  sudo apt-get install -y nodejs
fi
echo "[ok] Node $(node -v)"

# ── systemd: tasksy-admin.service ────────────────────────────────────────────
sudo tee /etc/systemd/system/tasksy-admin.service > /dev/null <<'SERVICE'
[Unit]
Description=Tasksy Admin Panel (Next.js)
After=network.target

[Service]
Type=simple
User=ubuntu
WorkingDirectory=/home/ubuntu/tasksy-admin
EnvironmentFile=/home/ubuntu/tasksy-admin/.env
Environment=NODE_ENV=production
Environment=PORT=3000
Environment=HOSTNAME=127.0.0.1
ExecStart=/usr/bin/node /home/ubuntu/tasksy-admin/server.js
Restart=always
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=tasksy-admin

[Install]
WantedBy=multi-user.target
SERVICE

sudo systemctl daemon-reload
sudo systemctl enable tasksy-admin.service
echo "[ok] tasksy-admin.service registered"

# ── Nginx config ─────────────────────────────────────────────────────────────
NGINX_CONF="/etc/nginx/sites-available/tasksy-admin.conf"
NGINX_ENABLED="/etc/nginx/sites-enabled/tasksy-admin.conf"

# Copy the config from this repo (assumes repo is checked out next to this script)
REPO_CONF="$(dirname "$0")/../nginx/tasksy-admin.conf"
if [ -f "$REPO_CONF" ]; then
  sudo cp "$REPO_CONF" "$NGINX_CONF"
  echo "[ok] Nginx config copied to $NGINX_CONF"
else
  echo "[warn] nginx/tasksy-admin.conf not found — copy it manually to $NGINX_CONF"
fi

if [ ! -L "$NGINX_ENABLED" ]; then
  sudo ln -s "$NGINX_CONF" "$NGINX_ENABLED"
  echo "[ok] Nginx site enabled"
fi

# Test Nginx config
sudo nginx -t && echo "[ok] Nginx config valid"

echo ""
echo "============================================================"
echo " Next steps:"
echo "  1. Edit $NGINX_CONF — replace 'admin.yourdomain.com'"
echo "  2. sudo systemctl reload nginx"
echo "  3. (Optional) sudo certbot --nginx -d admin.yourdomain.com"
echo "  4. Push to 'working' branch to trigger the CI/CD deploy"
echo "============================================================"
