# Project Aurora — Production Deployment Guide

> **Platform Version**: `1.0.0`  
> **Supported Environments**: Bare-Metal Linux, Dedicated KVM Servers, Multi-Cloud (AWS, Hetzner, OVH, GCP, Azure)  
> **Target Operating Systems**: Ubuntu 24.04 LTS / 22.04 LTS, Debian 12 Bookworm  
> **Virtualization Engine**: Incus 6.x (Linux Containers & KVM Virtual Machines)  
> **Automated Quick Installer**: `sudo ./install.sh` (or `sudo ./install.sh --help`)

---

## ⚡ Automated Quick Installation

For automated production deployments with interactive menus, role selection, auto-configuration of PostgreSQL, Incus, Nginx, Let's Encrypt SSL, and systemd services, run the all-in-one installer script:

```bash
git clone https://github.com/aurora-vm/aurora.git /opt/aurora
cd /opt/aurora
sudo ./install.sh
```

Or deploy directly via role flags:
- **Control Plane**: `sudo ./install.sh --role control-plane --domain aurora.example.com --email admin@example.com`
- **Hypervisor Node Agent**: `sudo ./install.sh --role agent --hub-addr aurora.example.com:9443 --enrollment-token <TOKEN>`
- **All-in-One Standalone**: `sudo ./install.sh --role all-in-one`

For complete manual step-by-step installation instructions, follow the sections below.

---

## Table of Contents

1. [Production Architecture](#1-production-architecture)
2. [Hardware & Software Prerequisites](#2-hardware--software-prerequisites)
3. [PostgreSQL Setup & Migrations](#3-postgresql-setup--migrations)
4. [S3 / Cloudflare R2 Object Storage Setup](#4-s3--cloudflare-r2-object-storage-setup)
5. [Control Plane Installation (`aurora-server`)](#5-control-plane-installation-aurora-server)
6. [Node Agent Installation & Enrollment (`aurora-agent`)](#6-node-agent-installation--enrollment-aurora-agent)
7. [PKI & Mutual TLS (mTLS) Architecture](#7-pki--mutual-tls-mtls-architecture)
8. [Frontend Deployment (Customer & Admin Portals)](#8-frontend-deployment-customer--admin-portals)
9. [Reverse Proxy & TLS Termination (Nginx & Caddy)](#9-reverse-proxy--tls-termination-nginx--caddy)
10. [Environment & Configuration Variables](#10-environment--configuration-variables)
11. [Systemd Service Units](#11-systemd-service-units)
12. [Firewall & Network Architecture](#12-firewall--network-architecture)
13. [DNS Configuration](#13-dns-configuration)
14. [Initial Superadmin Bootstrap Procedure](#14-initial-superadmin-bootstrap-procedure)
15. [Backup Configuration & Disaster Recovery Policies](#15-backup-configuration--disaster-recovery-policies)
16. [Health Checks, Prometheus Metrics & Alerting](#16-health-checks-prometheus-metrics--alerting)
17. [First Workload Deployment Verification](#17-first-workload-deployment-verification)
18. [Disaster Recovery Verification Protocol](#18-disaster-recovery-verification-protocol)
19. [Production Security Hardening Checklist](#19-production-security-hardening-checklist)
20. [Troubleshooting & Runbook Reference](#20-troubleshooting--runbook-reference)

---

## 1. Production Architecture

Project Aurora follows a **Decoupled Control-Plane / Distributed Hypervisor Node Architecture**:

```
                         [ INTERNET / CUSTOMERS / ADMINS ]
                                        │
                                        │ HTTPS (443) / WSS
                                        ▼
                      ┌───────────────────────────────────┐
                      │    Reverse Proxy (Nginx / Caddy)  │
                      │   TLS Termination + WSS Routing   │
                      └─────────────────┬─────────────────┘
                                        │
             ┌──────────────────────────┴──────────────────────────┐
             │ HTTP (:8080)                                        │ Static Assets / SPA
             ▼                                                     ▼
┌──────────────────────────────────────────────┐        ┌───────────────────────┐
│     Aurora Control Plane (aurora-server)     │◄───────┤ React 18 SPA (web/dist│
│                                              │        └───────────────────────┘
│  • REST API & Auth / RBAC Engine             │
│  • WebSocket Console/VNC & Event Stream      │
│  • Durable Job Queue (FOR UPDATE SKIP LOCKED)│
│  • Scheduler & Placement Optimizer           │
│  • Billing, Quotas & Metering Engine         │
│  • Internal PKI Certificate Authority        │
│  • State Reconciler & Self-Healing Engine    │
│  • Disaster Recovery Coordinator             │
└──────────────┬─────────────────┬─────────────┘
               │                 │
               │ PostgreSQL      │ gRPC mTLS Tunnel (:9443)
               ▼                 ▼
   ┌──────────────────────┐  ┌─────────────────────────────────────────────────┐
   │ PostgreSQL 16 Pool   │  │ Hypervisor Nodes (aurora-agent + Incus 6.x)     │
   │ (12 Up/Down Schemas) │  │                                                 │
   │                      │  │ ┌───────────────┐ ┌───────────────┐ ┌─────────┐ │
   │ • Identities & Roles │  │ │ Node Agent 01 │ │ Node Agent 02 │ │ Node 03 │ │
   │ • Workload Topology  │  │ └───────┬───────┘ └───────┬───────┘ └────┬────┘ │
   │ • IPAM & Allocations │  │         │ Unix Socket     │              │      │
   │ • SHA-256 Audit Logs │  │         ▼                 ▼              ▼      │
   │ • Jobs & Migrations  │  │ ┌─────────────────────────────────────────────┐ │
   │ • Metering & Invoices│  │ │ Incus Daemon (/var/lib/incus/unix.socket)    │ │
   │ • DR Backup Records  │  │ │ • ZFS / Btrfs / LVM Storage Pools           │ │
   └──────────────────────┘  │ │ • Linux Containers (LXC) & KVM QEMU VMs     │ │
                             │ │ • Bridged / OVN Virtual Networks            │ │
                             │ └─────────────────────────────────────────────┘ │
                             └─────────────────────────────────────────────────┘
```

---

## 2. Hardware & Software Prerequisites

### Control Plane Node
- **CPU**: 2+ vCPU (x86_64 or arm64)
- **RAM**: 4 GB minimum (8 GB recommended for production workloads)
- **Disk**: 40 GB NVMe/SSD (root + database storage)
- **OS**: Ubuntu 24.04 LTS, Ubuntu 22.04 LTS, or Debian 12 Bookworm
- **Software**:
  - `git`, `curl`, `jq`, `tar`, `ca-certificates`
  - `golang` >= 1.22 (if compiling from source)
  - `nodejs` >= 20.x and `npm` >= 10.x (if compiling frontend)
  - `postgresql-15` or `postgresql-16`

### Hypervisor Nodes (Compute Nodes)
- **CPU**: 4+ Physical Cores with Hardware Virtualization (`VT-x` or `AMD-V` enabled)
  - Check with: `egrep -c '(vmx|svm)' /proc/cpuinfo` (must return > 0)
  - Verify KVM device: `ls -la /dev/kvm`
- **RAM**: 16 GB to 512 GB depending on tenant capacity
- **Disk**: 200 GB+ dedicated NVMe storage pool for container & VM root disks
- **OS**: Ubuntu 24.04 LTS / 22.04 LTS (recommended) or Debian 12
- **Kernel Modules Required**:
  ```bash
  sudo modprobe kvm
  sudo modprobe kvm_intel # or kvm_amd
  sudo modprobe vhost_net
  sudo modprobe zfs       # if using ZFS storage pool
  ```
- **Virtualization Daemon**:
  - Incus >= 6.0 LTS (`incus`, `incus-admin` tools)

---

## 3. PostgreSQL Setup & Migrations

Aurora uses PostgreSQL with connection pooling via `pgxpool` and requires the `uuid-ossp` and `citext` extensions.

### 3.1 Install & Configure PostgreSQL 16 (Ubuntu/Debian)

```bash
# 1. Install PostgreSQL 16
sudo apt update
sudo apt install -y postgresql-16 postgresql-contrib

# 2. Create Aurora database and dedicated user
sudo -u postgres psql << 'EOF'
CREATE USER aurora WITH ENCRYPTED PASSWORD 'YourStrongProductionPasswordHere!';
CREATE DATABASE aurora OWNER aurora;
\c aurora
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "citext";
GRANT ALL PRIVILEGES ON DATABASE aurora TO aurora;
GRANT ALL ON SCHEMA public TO aurora;
EOF

# 3. Configure Connection Security in /etc/postgresql/16/main/pg_hba.conf
# (Allow localhost or VPC subnet connections using scram-sha-256)
echo "host aurora aurora 127.0.0.1/32 scram-sha-256" | sudo tee -a /etc/postgresql/16/main/pg_hba.conf
sudo systemctl restart postgresql
```

### 3.2 Automated Database Migrations
Aurora includes 12 versioned migration pairs in the [`migrations/`](file:///home/lumi/Downloads/OVM%20V2/OVM%20V2/migrations) directory. When `AURORA_AUTO_MIGRATE=true` (default), the control plane automatically applies pending migrations at boot:

```bash
# Verify migrations table in PostgreSQL after initial boot:
PGPASSWORD='YourStrongProductionPasswordHere!' psql -U aurora -d aurora -h 127.0.0.1 -c "SELECT * FROM schema_migrations ORDER BY version ASC;"
```

| Migration ID | Subsystem Schema |
|---|---|
| `000001_initial_schema` | Core users, roles, permissions, locations, nodes, enrollments |
| `000002_identity_and_sessions` | JWT refresh tokens, API keys, TOTP secrets, user preferences |
| `000003_instances_schema` | Compute instances, specs, guest files, power states |
| `000004_ipam_and_networking` | Subnet pools, IP allocations, MAC bindings, firewall rules |
| `000005_storage_and_volumes` | ZFS/Btrfs storage pools, persistent block volumes, snapshots |
| `000006_monitoring_and_telemetry` | Telemetry ring buffer, threshold alert rules, alert history |
| `000007_audit_and_compliance` | Tamper-evident SHA-256 audit ledger, SIEM forwarders |
| `000008_templates_and_images` | OS template registry, Cloud-Init configs, image fingerprints |
| `000009_billing_and_usage` | Pricing plans, subscriptions, quotas, metered usage, invoices |
| `000010_events_and_notifications` | In-app notifications, preferences, HMAC-SHA256 webhooks, deliveries |
| `000011_operations_ha_and_jobs` | Durable job queues, worker leases, workload live migrations |
| `000012_disaster_recovery_and_hardening`| Encrypted backups, recovery points, DR restore plans, key rotations |

---

## 4. S3 / Cloudflare R2 Object Storage Setup

Aurora stores encrypted backup artifacts and cached base OS images using an AES-256-GCM envelope encryption layer.

### 4.1 Master Encryption & JWT Secrets Generation

Generate cryptographically secure 256-bit and 512-bit production secrets:

```bash
# 1. Generate 32 bytes (64 hex characters) for AURORA_MASTER_KEY (Envelope AES-256-GCM)
openssl rand -hex 32
# Output example: a3f89e2c4189b3f07a6d8120c8b934e6f1a7d8c90321456789abcdef01234567

# 2. Generate 64 bytes (128 hex characters) for AURORA_JWT_SECRET (HMAC-SHA256 Session Signing)
openssl rand -hex 64
# Output example: e7b419c8f05a2d619b84e3c781d0f5a6b2c4e9f1a7d8c90321456789abcdef01e7b419c8f05a2d619b84e3c781d0f5a6b2c4e9f1a7d8c90321456789abcdef01
```
Store these values in `/etc/aurora/server.env`.

### 4.2 Cloudflare R2 / AWS S3 Bucket Setup
Create a dedicated bucket for Aurora backups (e.g. `aurora-backups-production`):

```bash
# AWS CLI example:
aws s3api create-bucket \
  --bucket aurora-backups-production \
  --region us-east-1 \
  --object-ownership BucketOwnerEnforced

# Enable bucket versioning and 90-day lifecycle expiration:
aws s3api put-bucket-versioning \
  --bucket aurora-backups-production \
  --versioning-configuration Status=Enabled
```

---

## 5. Control Plane Installation (`aurora-server`)

### 5.1 System User and Directory Structure

```bash
# 1. Create system service user
sudo useradd --system --no-create-home --shell /usr/sbin/nologin aurora

# 2. Create required directories
sudo mkdir -p /etc/aurora
sudo mkdir -p /var/lib/aurora/tls
sudo mkdir -p /var/log/aurora

sudo chown -R aurora:aurora /var/lib/aurora /var/log/aurora
sudo chmod 700 /var/lib/aurora /var/lib/aurora/tls
```

### 5.2 Compiling & Installing Binaries

```bash
# Clone the production repository
git clone https://github.com/aurora-vm/aurora.git /opt/aurora
cd /opt/aurora

# Build production binaries with Git commit and build timestamp metadata
VERSION="1.0.0"
COMMIT=$(git rev-parse --short HEAD 2>/dev/null || echo "release")
BUILD_DATE=$(date -u +"%Y-%m-%dT%H:%M:%SZ")

CGO_ENABLED=0 go build \
  -ldflags="-w -s -X 'github.com/aurora-vm/aurora/pkg/version.Version=${VERSION}' -X 'github.com/aurora-vm/aurora/pkg/version.GitCommit=${COMMIT}' -X 'github.com/aurora-vm/aurora/pkg/version.BuildDate=${BUILD_DATE}'" \
  -o /usr/local/bin/aurora-server ./cmd/aurora-server

# Copy migration files for auto-migration runner
sudo cp -r migrations /etc/aurora/migrations
sudo chown -R aurora:aurora /etc/aurora
```

### 5.3 Configure Server Environment (`/etc/aurora/server.env`)

```bash
sudo tee /etc/aurora/server.env << 'EOF'
AURORA_ENV=production
AURORA_HTTP_PORT=8080
AURORA_GRPC_PORT=9443
AURORA_DATABASE_URL=postgres://aurora:YourStrongProductionPasswordHere!@127.0.0.1:5432/aurora?sslmode=disable
AURORA_REDIS_URL=redis://127.0.0.1:6379/0
AURORA_MASTER_KEY=a3f89e2c4189b3f07a6d8120c8b934e6f1a7d8c90321456789abcdef01234567
AURORA_JWT_SECRET=e7b419c8f05a2d619b84e3c781d0f5a6b2c4e9f1a7d8c90321456789abcdef01e7b419c8f05a2d619b84e3c781d0f5a6b2c4e9f1a7d8c90321456789abcdef01
AURORA_LOG_LEVEL=info
AURORA_AUTO_MIGRATE=true
EOF

sudo chmod 600 /etc/aurora/server.env
sudo chown aurora:aurora /etc/aurora/server.env
```

---

## 6. Node Agent Installation & Enrollment (`aurora-agent`)

Each hypervisor node runs `aurora-agent` alongside the local Incus daemon.

### 6.1 Install & Initialize Incus 6.x

```bash
# 1. Configure official Zabbly Incus LTS 6.0 repository (recommended for latest Incus 6.x / required on Debian & Ubuntu 22.04)
sudo mkdir -p /etc/apt/keyrings
sudo curl -fsSL https://pkgs.zabbly.com/key.asc -o /etc/apt/keyrings/zabbly.asc

sudo tee /etc/apt/sources.list.d/zabbly-incus-lts-6.0.sources << 'EOF'
Enabled: yes
Types: deb
URIs: https://pkgs.zabbly.com/incus/lts-6.0
Suites: noble
Components: main
Architectures: amd64
Signed-By: /etc/apt/keyrings/zabbly.asc
EOF
# (Replace 'noble' and 'amd64' with your release codename and architecture if different)

# 2. Install Incus and storage utilities
sudo apt update
sudo apt install -y incus incus-tools zfsutils-linux bridge-utils

# 3. Add root / service users to the incus-admin group
sudo usermod -aG incus-admin aurora

# 4. Initialize Incus storage pool and bridge network
sudo incus admin init --auto \
  --storage-backend zfs \
  --storage-pool default \
  --network-address 127.0.0.1 \
  --network-port 8443
```

### 6.2 Compile and Install `aurora-agent` Binary

```bash
# If compiling directly on the hypervisor host:
cd /opt/aurora
CGO_ENABLED=0 go build \
  -ldflags="-w -s -X 'github.com/aurora-vm/aurora/pkg/version.Version=1.0.0'" \
  -o /usr/local/bin/aurora-agent ./cmd/aurora-agent

# Set executable permissions and directory structure:
sudo mkdir -p /var/lib/aurora/tls /etc/aurora
sudo chmod +x /usr/local/bin/aurora-agent
sudo chmod 700 /var/lib/aurora /var/lib/aurora/tls
```

### 6.3 Generate Enrollment Token (from Admin API)

From an administrator terminal or the Aurora Admin Portal (`/admin/nodes`):

```bash
# Generate single-use enrollment token (valid for 1 hour):
curl -s -X POST "https://aurora.example.com/api/v1/nodes/enrollment-tokens" \
  -H "Authorization: Bearer <ADMIN_JWT_TOKEN>" \
  -H "Content-Type: application/json" \
  -d '{
    "locationId": "loc-eu-central-1",
    "nodeNamePattern": "hv-01.infra.example.com",
    "ttlSeconds": 3600
  }' | jq .
```

### 6.4 Execute Enrollment & Tunnel Connection

Configure the agent environment with the enrollment token:

```bash
sudo tee /etc/aurora/agent.env << 'EOF'
AURORA_ENV=production
AURORA_NODE_NAME=hv-01
AURORA_NODE_FQDN=hv-01.infra.example.com
AURORA_HUB_ADDRESS=aurora.example.com:9443
AURORA_HUB_HTTP_ADDRESS=https://aurora.example.com
AURORA_ENROLLMENT_TOKEN=aurora_enroll_xxxxxxxxxxxxxx
AURORA_STATE_DIR=/var/lib/aurora
AURORA_INCUS_SOCKET=/var/lib/incus/unix.socket
AURORA_DRIVER=socket
AURORA_LOG_LEVEL=info
EOF

sudo chmod 600 /etc/aurora/agent.env
```

Start the agent (manually or via systemd). The agent will:
1. Generate an ECDSA P-256 private key and Certificate Signing Request (CSR).
2. Submit the CSR to `POST /api/v1/nodes/enroll`.
3. Receive the signed node certificate and Root CA certificate.
4. Save them to `/var/lib/aurora/tls/node.crt`, `/var/lib/aurora/tls/node.key`, `/var/lib/aurora/tls/ca.crt`.
5. Establish an outbound persistent mTLS gRPC tunnel to `aurora.example.com:9443`.

---

## 7. PKI & Mutual TLS (mTLS) Architecture

Aurora operates a built-in Certificate Authority (`internal/infra/pki/ca.go`) using **ECDSA P-256** and **SHA-256**:

```
                 ┌────────────────────────────────┐
                 │  Aurora Internal Root CA       │
                 │  ECDSA P-256 (10-Year TTL)     │
                 └──────────────┬─────────────────┘
                                │
        ┌───────────────────────┴───────────────────────┐
        │ Issues at Boot                                │ Signs during Enrollment
        ▼                                               ▼
┌─────────────────────────────────┐   ┌───────────────────────────────────┐
│ Control Plane Gateway Server    │   │ Hypervisor Node Agent Certificate │
│ Certificate (gRPC :9443)        │   │ (mTLS Client Identity)            │
│ SAN: [127.0.0.1, FQDN]          │   │ CN: node-<node-id>                │
└─────────────────────────────────┘   └───────────────────────────────────┘
```

- **Server Authentication**: Node agents verify the server's TLS certificate against the enrolled Root CA.
- **Client Authentication**: The control plane requires and verifies client certificates on all gRPC connections (`tls.RequireAndVerifyClientCert`).
- **Certificate Revocation**: When an administrator revokes a node (`POST /api/v1/nodes/:id/revoke`), the node's certificate fingerprint is permanently blacklisted.

---

## 8. Frontend Deployment (Customer & Admin Portals)

The frontend is a React 18 Single Page Application built with Vite and Tailwind CSS.

### 8.1 Compile Production Assets

```bash
cd /opt/aurora/web

# 1. Install dependencies
npm ci

# 2. Run unit tests
npm test -- --run

# 3. Build optimized production bundle
npm run build
# Assets compiled to /opt/aurora/web/dist
```

### 8.2 Serving Options
- **Option A (Built-in SPA Handler)**: `aurora-server` automatically detects and serves `/opt/aurora/web/dist` on port `:8080`.
- **Option B (Recommended — Direct Reverse Proxy Serving)**: Nginx/Caddy serves static files directly from `/var/www/aurora/dist` and proxies only `/api/`, `/healthz`, `/readyz`, and `/metrics` to Go.

```bash
# Copy distribution files to web root
sudo mkdir -p /var/www/aurora
sudo cp -r /opt/aurora/web/dist /var/www/aurora/dist
sudo chown -R www-data:www-data /var/www/aurora
```

---

## 9. Reverse Proxy & TLS Termination (Nginx & Caddy)

### 9.1 Production Nginx Configuration

#### Step 1: Install Nginx & Certbot

```bash
sudo apt update
sudo apt install -y nginx certbot python3-certbot-nginx

# Obtain SSL Certificate via Let's Encrypt
sudo certbot certonly --nginx -d aurora.example.com
```

#### Step 2: Write Nginx Configuration (`/etc/nginx/sites-available/aurora.conf`)

```bash
sudo tee /etc/nginx/sites-available/aurora.conf << 'EOF'
# Map for WebSocket Connection Upgrade
map $http_upgrade $connection_upgrade {
    default upgrade;
    ''      close;
}

# HTTP -> HTTPS Redirect
server {
    listen 80;
    listen [::]:80;
    server_name aurora.example.com;

    location /.well-known/acme-challenge/ {
        root /var/www/certbot;
    }

    location / {
        return 301 https://$host$request_uri;
    }
}

# HTTPS Primary Virtual Host
server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name aurora.example.com;

    # SSL Certificates (Let's Encrypt / Certbot)
    ssl_certificate /etc/letsencrypt/live/aurora.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/aurora.example.com/privkey.pem;

    # Modern TLS Security Parameters
    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers 'ECDHE-ECDSA-AES128-GCM-SHA256:ECDHE-RSA-AES128-GCM-SHA256:ECDHE-ECDSA-AES256-GCM-SHA384:ECDHE-RSA-AES256-GCM-SHA384:DHE-RSA-AES128-GCM-SHA256:DHE-RSA-AES256-GCM-SHA384';
    ssl_prefer_server_ciphers off;
    ssl_session_timeout 1d;
    ssl_session_cache shared:SSL:10m;
    ssl_session_tickets off;

    # Security Headers
    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;
    add_header X-XSS-Protection "1; mode=block" always;
    add_header Referrer-Policy "strict-origin-when-cross-origin" always;

    # Maximum file upload size (for OS images and recovery artifacts)
    client_max_body_size 50G;

    # 1. API Endpoints
    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 300s;
    }

    # 2. Interactive WebSocket Consoles (Terminal PTY & VNC) & Event Stream
    location ~* ^/api/v1/(instances/[^/]+/console/(exec|vnc)|events/stream) {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection $connection_upgrade;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
        proxy_read_timeout 86400s; # Keep terminal sessions alive
        proxy_send_timeout 86400s;
    }

    # 3. Health & Readiness Probes
    location ~* ^/(healthz|readyz|health/|metrics) {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
    }

    # 4. Frontend Single Page Application (Static Files)
    root /var/www/aurora/dist;
    index index.html;

    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
        try_files $uri =404;
    }

    location / {
        try_files $uri $uri/ /index.html;
    }
}
EOF
```

#### Step 3: Enable Nginx Virtual Host & Reload

```bash
sudo ln -sf /etc/nginx/sites-available/aurora.conf /etc/nginx/sites-enabled/
sudo rm -f /etc/nginx/sites-enabled/default
sudo nginx -t
sudo systemctl restart nginx
sudo systemctl enable nginx
```

### 9.2 Alternative: Production Caddy Configuration

#### Step 1: Install Caddy

```bash
sudo apt update
sudo apt install -y debian-keyring debian-archive-keyring apt-transport-https curl
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/gpg.key' | sudo gpg --dearmor -o /usr/share/keyrings/caddy-stable-archive-keyring.gpg
curl -1sLf 'https://dl.cloudsmith.io/public/caddy/stable/debian.deb.txt' | sudo tee /etc/apt/sources.list.d/caddy-stable.list
sudo apt update
sudo apt install -y caddy
```

#### Step 2: Write Caddyfile (`/etc/caddy/Caddyfile`)

```bash
sudo tee /etc/caddy/Caddyfile << 'EOF'
aurora.example.com {
    # Automatic Let's Encrypt TLS
    encode gzip zstd

    # Security Headers
    header {
        Strict-Transport-Security "max-age=63072000; includeSubDomains; preload"
        X-Frame-Options "DENY"
        X-Content-Type-Options "nosniff"
        Referrer-Policy "strict-origin-when-cross-origin"
    }

    request_body {
        max_size 50GB
    }

    # Proxy API & WebSockets
    handle /api/* {
        reverse_proxy 127.0.0.1:8080
    }

    handle /healthz {
        reverse_proxy 127.0.0.1:8080
    }

    handle /readyz {
        reverse_proxy 127.0.0.1:8080
    }

    handle /metrics {
        reverse_proxy 127.0.0.1:8080
    }

    # SPA Frontend Static Serving
    root * /var/www/aurora/dist
    file_server
    try_files {path} /index.html
}
EOF

sudo systemctl restart caddy
sudo systemctl enable caddy
```

---

## 10. Environment & Configuration Variables

### 10.1 Control Plane Variables (`/etc/aurora/server.env`)

| Variable | Type | Default | Description |
|---|---|---|---|
| `AURORA_ENV` | `string` | `development` | Runtime environment: `production` or `development` |
| `AURORA_HTTP_PORT` | `int` | `8080` | Internal port for REST API, WebSockets, SPA and Probes |
| `AURORA_GRPC_PORT` | `int` | `9443` | Port for internal gRPC mTLS Hub (avoid 8443 Incus conflict) |
| `AURORA_DATABASE_URL` | `string` | *(required)* | PostgreSQL connection URI (`postgres://user:pass@host:5432/db?sslmode=...`) |
| `AURORA_REDIS_URL` | `string` | `redis://localhost:6379/0` | Optional Redis / Valkey connection URI |
| `AURORA_MASTER_KEY` | `string` | *(required)* | 32-byte (64 hex char) master key for AES-256-GCM data encryption |
| `AURORA_JWT_SECRET` | `string` | *(required)* | 64-byte high-entropy secret for HMAC-SHA256 token signing |
| `AURORA_LOG_LEVEL` | `string` | `info` | Logging verbosity: `debug`, `info`, `warn`, `error` |
| `AURORA_AUTO_MIGRATE` | `bool` | `true` | Auto-apply pending database schema migrations on boot |

### 10.2 Node Agent Variables (`/etc/aurora/agent.env`)

| Variable | Type | Default | Description |
|---|---|---|---|
| `AURORA_ENV` | `string` | `development` | Runtime environment (`production`) |
| `AURORA_NODE_NAME` | `string` | `localhost-node` | Unique identifier for hypervisor node |
| `AURORA_NODE_FQDN` | `string` | *(hostname)* | Fully Qualified Domain Name of hypervisor host |
| `AURORA_HUB_ADDRESS` | `string` | `127.0.0.1:9443` | Control plane gRPC endpoint (`host:port`) |
| `AURORA_HUB_HTTP_ADDRESS`| `string` | `http://127.0.0.1:8080`| Control plane HTTPS URL for enrollment CSR submission |
| `AURORA_ENROLLMENT_TOKEN`| `string` | `""` | Single-use token for initial cryptographic enrollment |
| `AURORA_STATE_DIR` | `string` | `/var/lib/aurora` | Local directory for storing mTLS keys and node state |
| `AURORA_INCUS_SOCKET` | `string` | `/var/lib/incus/unix.socket` | Unix socket path to local Incus daemon |
| `AURORA_DRIVER` | `string` | `socket` | Hypervisor driver: `socket` (Incus) or `simulated` |
| `AURORA_LOG_LEVEL` | `string` | `info` | Logging verbosity: `debug`, `info`, `warn`, `error` |

---

## 11. Systemd Service Units

### 11.1 Control Plane Service (`/etc/systemd/system/aurora-server.service`)

```bash
sudo tee /etc/systemd/system/aurora-server.service << 'EOF'
[Unit]
Description=Project Aurora Virtualization Control Plane
Documentation=https://github.com/aurora-vm/aurora
After=network.target postgresql.service
Wants=postgresql.service

[Service]
Type=simple
User=aurora
Group=aurora
WorkingDirectory=/var/lib/aurora
EnvironmentFile=/etc/aurora/server.env
ExecStart=/usr/local/bin/aurora-server
Restart=always
RestartSec=5s

# Security Sandboxing & Resource Limits
LimitNOFILE=65536
LimitNPROC=32768
ProtectSystem=full
ProtectHome=true
NoNewPrivileges=true
PrivateTmp=true

# Standard Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=aurora-server

[Install]
WantedBy=multi-user.target
EOF
```

### 11.2 Hypervisor Node Agent Service (`/etc/systemd/system/aurora-agent.service`)

```bash
sudo tee /etc/systemd/system/aurora-agent.service << 'EOF'
[Unit]
Description=Project Aurora Hypervisor Node Agent
Documentation=https://github.com/aurora-vm/aurora
After=network.target incus.service
Wants=incus.service

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=/var/lib/aurora
EnvironmentFile=/etc/aurora/agent.env
ExecStart=/usr/local/bin/aurora-agent
Restart=always
RestartSec=5s

# Security Sandboxing & Resource Limits
LimitNOFILE=65536
LimitNPROC=32768

# Standard Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=aurora-agent

[Install]
WantedBy=multi-user.target
EOF
```

### 11.3 Enable and Start Services

```bash
# 1. On the Control Plane:
sudo systemctl daemon-reload
sudo systemctl enable --now aurora-server

# Verify Control Plane Status and Live Logs:
sudo systemctl status aurora-server
sudo journalctl -u aurora-server -n 50 --no-pager

# 2. On Hypervisor Nodes:
sudo systemctl daemon-reload
sudo systemctl enable --now aurora-agent

# Verify Hypervisor Agent Status and Live Logs:
sudo systemctl status aurora-agent
sudo journalctl -u aurora-agent -n 50 --no-pager
```

---

## 12. Firewall & Network Architecture

```
                    ┌─────────────────────────┐
                    │   INCOMING TRAFFIC      │
                    └───────────┬─────────────┘
                                │
       ┌────────────────────────┼────────────────────────┐
       │ Port 80 (HTTP)         │ Port 443 (HTTPS)       │ Port 9443 (gRPC mTLS)
       ▼                        ▼                        ▼
┌──────────────┐         ┌──────────────┐         ┌──────────────┐
│ ACME / HTTP  │         │ Nginx Proxy  │         │ aurora-server│
│ Certbot Auth │         │ REST / SPA   │         │ Node Tunnels │
└──────────────┘         └──────────────┘         └──────────────┘
```

### 12.1 Control Plane Firewall Configuration (UFW)

```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing

# Public Web Ports
sudo ufw allow 80/tcp comment "HTTP Let's Encrypt"
sudo ufw allow 443/tcp comment "HTTPS Public API and Web Portal"

# Dedicated Hypervisor gRPC Port
sudo ufw allow 9443/tcp comment "Aurora gRPC mTLS Hub"

# SSH Management (Restricted to Management Subnet)
sudo ufw allow from 198.51.100.0/24 to any port 22 proto tcp comment "SSH Admin"

sudo ufw enable
```

### 12.2 Hypervisor Node Firewall Configuration

```bash
sudo ufw default deny incoming
sudo ufw default allow outgoing

# Allow Outbound Connections to Control Plane
# (Hypervisors initiate outbound mTLS to aurora.example.com:9443 and :443)

# Allow Routed/Bridged Traffic for Guest Containers and VMs
sudo ufw allow in on incusbr0
sudo ufw route allow in on incusbr0
sudo ufw route allow out on incusbr0

sudo ufw enable
```

---

## 13. DNS Requirements

| Record Type | Hostname | Target | Purpose |
|---|---|---|---|
| `A` / `AAAA` | `aurora.example.com` | Control Plane Public IP | Customer & Admin Web Portal & REST API |
| `A` / `AAAA` | `grpc.aurora.example.com` | Control Plane Public IP | (Optional) Dedicated gRPC mTLS endpoint |
| `A` / `AAAA` | `hv-01.infra.example.com` | Hypervisor 01 Public IP | Node FQDN for telemetry & console proxy |
| `A` / `AAAA` | `hv-02.infra.example.com` | Hypervisor 02 Public IP | Node FQDN for telemetry & console proxy |

---

## 14. Initial Superadmin Bootstrap Procedure

Aurora uses an **Automated First-User Superadmin Initialization** model. The first user created through the registration endpoint is automatically granted the `superadmin` role with global scope (`*` permissions).

### 14.1 Bootstrap via REST API

```bash
# 1. Register the Initial Superadmin Account
BOOTSTRAP_RES=$(curl -s -X POST "https://aurora.example.com/api/v1/auth/register" \
  -H "Content-Type: application/json" \
  -d '{
    "username": "superadmin",
    "email": "security@example.com",
    "password": "CorrectHorseBatteryStaple2026!"
  }')

echo "$BOOTSTRAP_RES" | jq .

# 2. Authenticate and Obtain JWT Access Token
LOGIN_RES=$(curl -s -X POST "https://aurora.example.com/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{
    "usernameOrEmail": "superadmin",
    "password": "CorrectHorseBatteryStaple2026!"
  }')

ADMIN_TOKEN=$(echo "$LOGIN_RES" | jq -r .data.tokens.accessToken)
echo "Superadmin Access Token: $ADMIN_TOKEN"

# 3. Verify Superadmin Permissions
curl -s "https://aurora.example.com/api/v1/account" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .data.roles
# Returns: ["superadmin"]
```

> **Security Note**: Subsequent registrations from the web portal or API automatically receive the default `customer` role with strict tenant-scoped isolation.

---

## 15. Backup Configuration & Disaster Recovery Policies

### 15.1 Scheduled PostgreSQL State Backups (`pg_dump`)

```bash
# 1. Write automated PostgreSQL backup script
sudo tee /usr/local/bin/aurora-pg-backup.sh << 'EOF'
#!/usr/bin/env bash
set -e

BACKUP_DIR="/var/backups/aurora"
TIMESTAMP=$(date -u +"%Y%m%d_%H%M%SZ")
mkdir -p "$BACKUP_DIR"

# Dump database with SHA-256 checksum
PGPASSWORD='YourStrongProductionPasswordHere!' pg_dump -U aurora -h 127.0.0.1 -Fc aurora > "$BACKUP_DIR/aurora_db_$TIMESTAMP.dump"
sha256sum "$BACKUP_DIR/aurora_db_$TIMESTAMP.dump" > "$BACKUP_DIR/aurora_db_$TIMESTAMP.dump.sha256"

# Retain local backups for 14 days
find "$BACKUP_DIR" -type f -name "aurora_db_*.dump*" -mtime +14 -delete
EOF

# 2. Make script executable
sudo chmod +x /usr/local/bin/aurora-pg-backup.sh

# 3. Install Daily Cron Job (Runs at 02:00 UTC)
echo "0 2 * * * root /usr/local/bin/aurora-pg-backup.sh" | sudo tee /etc/cron.d/aurora-pg-backup
sudo chmod 644 /etc/cron.d/aurora-pg-backup
```

### 15.2 Control Plane Encrypted Disaster Recovery Points

Generate full cluster recovery points via the Admin API or Admin Portal (`/admin/recovery`):

```bash
curl -s -X POST "https://aurora.example.com/api/v1/admin/recovery/backups" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "resourceType": "cluster",
    "type": "full",
    "retentionDays": 90
  }' | jq .
```

---

## 16. Health Checks, Prometheus Metrics & Alerting

### 16.1 Health & Readiness Endpoints

- **Liveness Probe**: `GET https://aurora.example.com/healthz` (Returns `200 OK` when process is responsive).
- **Readiness Probe**: `GET https://aurora.example.com/readyz` (Returns `200 READY` when database pool is healthy).
- **Subsystem Diagnostic Health**: `GET https://aurora.example.com/api/v1/health` (Detailed component-level matrix).

### 16.2 Prometheus Scrape Configuration (`prometheus.yml`)

```yaml
scrape_configs:
  - job_name: 'aurora-control-plane'
    scrape_interval: 15s
    metrics_path: '/metrics'
    scheme: 'https'
    static_configs:
      - targets: ['aurora.example.com']
```

### 16.3 Alertmanager Rule Definition (`aurora_alerts.yml`)

```yaml
groups:
  - name: aurora.rules
    rules:
      - alert: AuroraNodeOffline
        expr: aurora_nodes_online_total < aurora_nodes_total
        for: 1m
        labels:
          severity: critical
        annotations:
          summary: "Aurora hypervisor node offline"
          description: "One or more Incus hypervisor nodes have stopped reporting heartbeats."

      - alert: AuroraJobQueueBacklog
        expr: aurora_jobs_pending_total > 50
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "Aurora job queue backlog high"
          description: "Over 50 asynchronous jobs are pending execution."
```

---

## 17. First Workload Deployment Verification

Follow this end-to-end verification sequence to confirm the entire cluster is functional:

### Step 1: Register an OS Template

```bash
curl -s -X POST "https://aurora.example.com/api/v1/templates" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "Ubuntu 24.04 LTS",
    "slug": "ubuntu-24.04",
    "description": "Canonical Ubuntu 24.04 Noble Numbat",
    "distribution": "ubuntu",
    "version": "24.04",
    "defaultUser": "ubuntu",
    "minCores": 1,
    "minMemoryBytes": 1073741824,
    "minStorageBytes": 10737418240,
    "supportedArchitectures": ["x86_64", "arm64"],
    "supportedTypes": ["container", "virtual-machine"],
    "isPublic": true,
    "cloudInit": {
      "user": "ubuntu",
      "allowPasswordAuth": true,
      "packageUpgrade": true,
      "packages": ["curl", "htop", "ufw", "qemu-guest-agent"]
    }
  }' | jq .
```

### Step 2: Provision a Container Workload

```bash
CREATE_RES=$(curl -s -X POST "https://aurora.example.com/api/v1/instances" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "prod-web-01",
    "type": "container",
    "cpuCores": 2,
    "memoryBytes": 2147483648,
    "storageBytes": 21474836480,
    "image": "images:ubuntu/24.04",
    "startAfterCreate": true
  }')

echo "$CREATE_RES" | jq .
INSTANCE_ID=$(echo "$CREATE_RES" | jq -r .data.id)
```

### Step 3: Verify Running State & Networking

```bash
sleep 3
curl -s "https://aurora.example.com/api/v1/instances/$INSTANCE_ID" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq '{id: .data.id, status: .data.status, ip: .data.ipv4Address, node: .data.nodeId}'
```

---

## 18. Disaster Recovery Verification Protocol

To verify the platform's ability to survive catastrophic loss, execute the 4-step DR pipeline:

```bash
# 1. Trigger DRY RUN Disaster Recovery Simulation
DRY_RUN_PLAN=$(curl -s -X POST "https://aurora.example.com/api/v1/admin/recovery/dry-run" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"backupId\":\"$BACKUP_ID\"}")
echo "$DRY_RUN_PLAN" | jq .

# 2. Trigger State Reconciliation (Dry Run)
curl -s -X POST "https://aurora.example.com/api/v1/admin/reconcile" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"dryRun": true}' | jq .

# 3. Execute Live Restoration
RESTORE_RES=$(curl -s -X POST "https://aurora.example.com/api/v1/admin/recovery/restore" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"backupId\":\"$BACKUP_ID\",\"confirmedDr\":true}")
echo "$RESTORE_RES" | jq .

# 4. Verify Cryptographic Audit Hash Chain
curl -s "https://aurora.example.com/api/v1/audit/verify" \
  -H "Authorization: Bearer $ADMIN_TOKEN" | jq .
# Expect: { "data": { "valid": true, "verifiedCount": ... } }
```

---

## 19. Production Security Hardening Checklist

- [ ] **Entropy Check**: `AURORA_JWT_SECRET` generated with at least 64 random bytes (`openssl rand -hex 64`).
- [ ] **Master Key Check**: `AURORA_MASTER_KEY` generated with 32 random bytes (`openssl rand -hex 32`).
- [ ] **TLS Everywhere**: Control plane HTTPS enforced with HTTP Strict Transport Security (HSTS).
- [ ] **mTLS Isolation**: Hypervisor gRPC port (`9443`) requires and verifies client certificates signed by internal CA.
- [ ] **PostgreSQL Security**: SSL connection mode enabled (`sslmode=verify-full`), non-default password, minimal user privileges.
- [ ] **Incus Daemon Socket**: `/var/lib/incus/unix.socket` owned by `root:incus-admin` with `0660` permissions.
- [ ] **Node State Directory**: `/var/lib/aurora/tls` restricted to `0700` permissions.
- [ ] **Tamper-Evident Ledger**: Audit chain verified periodically via cron or Prometheus alerts.
- [ ] **SSRF Defense**: Outbound webhook dispatcher verifies all destinations and blocks private/local IP ranges (`127.0.0.0/8`, `10.0.0.0/8`, `172.16.0.0/12`, `192.168.0.0/16`, `169.254.169.254`).
- [ ] **Rate Limiting**: Enabled across login and public endpoints (`transportHTTP.RateLimitMiddleware`).

---

## 20. Troubleshooting & Runbook Reference

| Symptom / Error | Root Cause | Resolution |
|---|---|---|
| `Failed to initialize node keystore: permission denied` | Agent user lacks write access to `/var/lib/aurora/tls` | `sudo chown -R root:root /var/lib/aurora && sudo chmod 700 /var/lib/aurora` |
| `Incus socket not found at /var/lib/incus/unix.socket` | Incus service is stopped or socket path differs | Run `sudo systemctl start incus` or set `AURORA_INCUS_SOCKET` |
| `rpc error: code = Unavailable desc = connection error` | Hypervisor cannot reach control plane on port 9443 | Verify UFW firewall rule: `sudo ufw allow 9443/tcp` on control plane |
| `checksum mismatch: expected ... got ...` | Backup artifact was corrupted or modified in object storage | Backup integrity protected. Restore from prior verified recovery point |
| `ErrCannotDeleteLastGoodBackup` | Attempting to delete the sole remaining verified backup | Protected point safety mechanism. Create a new backup before deleting |
| `WebSocket connection failed: 403 Forbidden` | `Origin` header rejected by WebSocket origin middleware | Ensure reverse proxy passes `Host` and `X-Forwarded-Proto` headers |
| `pq: relation "users" does not exist` | Database migrations have not been applied | Ensure `AURORA_AUTO_MIGRATE=true` or run `postgres.Migrator` |
| `JWT token has expired` | Client clock drift or access token expired (>15 min) | Refresh token via `POST /api/v1/auth/refresh` or sync system clocks with NTP |
