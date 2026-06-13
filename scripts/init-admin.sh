#!/bin/bash

# Initialize admin user for KubePilot

REMOTE_HOST="192.168.30.112"
REMOTE_USER="root"

echo "Creating admin user..."

ssh ${REMOTE_USER}@${REMOTE_HOST} "sudo -u postgres psql -d kubepilot -c \"
INSERT INTO roles (name, description, permissions, is_system, created_at, updated_at)
VALUES ('admin', 'System Administrator', '{}', true, NOW(), NOW())
ON CONFLICT (name) DO NOTHING;
\""

ssh ${REMOTE_USER}@${REMOTE_HOST} "sudo -u postgres psql -d kubepilot -c \"
INSERT INTO roles (name, description, permissions, is_system, created_at, updated_at)
VALUES ('user', 'Regular User', '{}', false, NOW(), NOW())
ON CONFLICT (name) DO NOTHING;
\""

ssh ${REMOTE_USER}@${REMOTE_HOST} "sudo -u postgres psql -d kubepilot -c \"
INSERT INTO roles (name, description, permissions, is_system, created_at, updated_at)
VALUES ('operator', 'Operator', '{}', false, NOW(), NOW())
ON CONFLICT (name) DO NOTHING;
\""

ssh ${REMOTE_USER}@${REMOTE_HOST} "sudo -u postgres psql -d kubepilot -c \"
INSERT INTO roles (name, description, permissions, is_system, created_at, updated_at)
VALUES ('viewer', 'Viewer', '{}', false, NOW(), NOW())
ON CONFLICT (name) DO NOTHING;
\""

# Note: The password hash below is for 'admin123' using bcrypt
# You may need to generate this hash using your application
ssh ${REMOTE_USER}@${REMOTE_HOST} "sudo -u postgres psql -d kubepilot -c \"
INSERT INTO users (username, email, password, real_name, status, role_id, created_at, updated_at)
SELECT 'admin', 'admin@kubepilot.io', '\$2a\$10\$N9qo8uLOickgx2ZMRZoMyeIjZAgcfl7p92ldGxad68LJZdL17lhWy', 'Administrator', 1, id, NOW(), NOW()
FROM roles WHERE name = 'admin'
ON CONFLICT (username) DO NOTHING;
\""

echo "Admin user created successfully!"
echo "Username: admin"
echo "Password: admin123"
