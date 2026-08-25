# Project Aurora
## Next-Generation Open-Source Distributed Virtualization & VPS Management Platform

Aurora is a modern, distributed, open-source virtualization management platform engineered from first principles as an alternative to commercial VPS panels.

---

## Monorepo Architecture Overview

```
.
├── cmd/
│   ├── aurora-server/       # Aurora Control Plane entrypoint (Hub)
│   └── aurora-agent/        # Aurora Node Agent entrypoint (Spoke daemon)
├── internal/
│   ├── app/                 # Application use cases, reconcilers, job handlers
│   ├── domain/              # Pure domain entities, value objects & interfaces
│   ├── infra/               # Infrastructure adapters (PostgreSQL, Redis, Incus, nftables, S3)
│   │   ├── config/          # Configuration loader
│   │   └── postgres/        # Database pool & SQL migration engine
│   └── transport/           # Transport layer (REST HTTP & gRPC mTLS)
│       ├── http/            # Chi router & HTTP API handlers
│       └── grpc/            # gRPC server & gateway implementations
├── pkg/                     # Exportable shared Go libraries
│   └── version/             # Build & version metadata
├── proto/                   # Protobuf workspace & definitions
│   └── aurora/v1/           # Shared Protobuf contracts (common, health, node)
├── gen/                     # Generated Go protobuf bindings
│   └── go/aurora/v1/
├── migrations/              # Versioned PostgreSQL SQL migrations (golang-migrate compatible)
├── web/                     # React 18 + TypeScript + Vite + Tailwind CSS Frontend
├── deployments/             # Docker Compose dev environment & production Dockerfiles
└── docs/                    # Architecture blueprint & documentation
```

---

## ⚡ Production Automated Installer

Project Aurora features an interactive, production-grade installer script for Ubuntu (24.04/22.04 LTS) and Debian (12/11):

```bash
# Clone and run interactive installer
git clone https://github.com/aurora-vm/aurora.git /opt/aurora
cd /opt/aurora
sudo ./install.sh
```

Or install specific roles non-interactively:
```bash
# Install Control Plane (API + Web Portal + PostgreSQL + Systemd)
sudo ./install.sh --role control-plane --domain aurora.example.com --email admin@example.com

# Install Hypervisor Node Agent (Incus 6.x + ZFS + mTLS Tunnel)
sudo ./install.sh --role agent --hub-addr aurora.example.com:9443 --enrollment-token <TOKEN>

# Install All-in-One Standalone (Control Plane + Node Agent on single server)
sudo ./install.sh --role all-in-one
```

---

## Quickstart & Local Development

### 1. Prerequisites
- **Go**: Version tracked to Incus LTS SDK (`go.mod` baseline 1.21+)
- **Node.js**: v18+ with `npm`
- **Protoc**: Protocol Buffers compiler (with `protoc-gen-go` and `protoc-gen-go-grpc`)
- **Docker**: For local PostgreSQL 16 & Valkey 7 development services

### 2. Start Development Databases
```bash
make dev-up
```

### 3. Build & Test Monorepo
```bash
# Generate Protobuf bindings, compile binaries, and execute all test suites
make all
```

### 4. Run the Control Plane Server
```bash
./bin/aurora-server
```
- **Liveness probe**: `http://localhost:8080/healthz`
- **API Health endpoint**: `http://localhost:8080/api/v1/health`
- **gRPC listener**: `localhost:8443`

### 5. Run the Frontend Dev Server
```bash
cd web
npm install
npm run dev
```
Open `http://localhost:3000` in your browser.

---

## License
Open-Source (Dual-License Model Ready).
