#!/bin/bash

# KubePilot Deployment Script
set -e

REMOTE_HOST="192.168.30.112"
REMOTE_USER="root"
REMOTE_DIR="/opt/kubepilot"

echo "=== KubePilot Deployment Script ==="
echo "Target: ${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_DIR}"

# Create remote directory
echo "1. Creating remote directories..."
ssh ${REMOTE_USER}@${REMOTE_HOST} "mkdir -p ${REMOTE_DIR}/{bin,configs,web,logs}"

# Transfer backend files
echo "2. Transferring backend files..."
scp bin/kubepilot ${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_DIR}/bin/
scp configs/config.yaml ${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_DIR}/configs/

# Transfer frontend files
echo "3. Transferring frontend files..."
scp -r web/* ${REMOTE_USER}@${REMOTE_HOST}:${REMOTE_DIR}/web/

# Create systemd service
echo "4. Creating systemd service..."
ssh ${REMOTE_USER}@${REMOTE_HOST} "cat > /etc/systemd/system/kubepilot.service << 'EOF'
[Unit]
Description=KubePilot - K8S Operations Management Platform
After=network.target postgresql.service redis.service

[Service]
Type=simple
User=root
WorkingDirectory=/opt/kubepilot
ExecStart=/opt/kubepilot/bin/kubepilot
Restart=always
RestartSec=5
LimitNOFILE=65536
Environment=KUBEPILOT_DATABASE_HOST=127.0.0.1
Environment=KUBEPILOT_REDIS_HOST=127.0.0.1

[Install]
WantedBy=multi-user.target
EOF"

# Reload systemd and start service
echo "5. Starting KubePilot service..."
ssh ${REMOTE_USER}@${REMOTE_HOST} "systemctl daemon-reload && systemctl enable kubepilot && systemctl restart kubepilot"

# Check service status
echo "6. Checking service status..."
ssh ${REMOTE_USER}@${REMOTE_HOST} "systemctl status kubepilot --no-pager"

echo ""
echo "=== Deployment Complete ==="
echo "Access KubePilot at: http://${REMOTE_HOST}:8080"
echo "Default login: admin / admin123"
