# Project Aurora — AI Agents & Developer Guide (`AGENTS.md`)

Welcome, AI agent / contributor! This document provides comprehensive operational guidelines, architectural rules, coding standards, and verification workflows for interacting with the **Project Aurora** codebase.

---

## 🧭 1. Repository Identity & Core Tech Stack

**Project Aurora** is an open-source, enterprise-grade distributed cloud virtualization control plane and hypervisor orchestration platform (managing Linux Containers / LXC and KVM Virtual Machines via Incus 6.x).

### Core Stack
- **Backend**: Go `1.23+` (Clean Architecture, DDD, Chi v5 Router, gRPC mTLS)
- **Virtualization Engine**: Incus 6.x (Linux Containers & KVM Virtual Machines)
- **Database**: PostgreSQL 16 (`github.com/jackc/pgx/v5` connection pooling & versioned migrations)
- **Contracts**: Protocol Buffers (`proto/aurora/v1/*.proto`) + Generated Go bindings (`gen/go/`)
- **Frontend SPA**: React 18, TypeScript, Vite, Tailwind CSS, Lucide Icons (`web/`)
- **CI / Automation**: GitHub Actions (`.github/workflows/ci.yml`), `act`, GNU Make, bash installer (`install.sh`)

---

## 🏗️ 2. Monorepo Directory Layout

```
.
├── cmd/
│   ├── aurora-server/       # Control Plane entrypoint (HTTP REST, WebSocket, gRPC mTLS Hub)
│   └── aurora-agent/        # Hypervisor Node Agent entrypoint (Spoke daemon, Incus bridge)
├── internal/
│   ├── app/                 # Application Use Cases, Job Handlers, Reconcilers, Health Supervisors
│   ├── domain/              # Pure Domain Entities, Value Objects, Domain Errors (ZERO external deps)
│   ├── infra/               # Infrastructure Adapters (Postgres, Incus SDK, Crypto, PKI, S3, JWT, TOTP)
│   └── transport/           # API Handlers (REST HTTP, Chi Router, WebSockets, gRPC Gateway)
├── pkg/
│   └── version/             # Global Version, Git Commit, and Build Metadata
├── proto/                   # Protobuf workspace specifications (common.proto, health.proto, node.proto)
├── gen/                     # Generated Go Protobuf bindings (DO NOT edit manually; run `make proto`)
├── migrations/              # 12 PostgreSQL SQL Migration pairs (000001 to 000012)
├── web/                     # React 18 Single Page Application (Customer & Admin Portals)
├── deployments/             # Docker Compose local dev environments & production configs
├── docs/                    # Architecture blueprints & operational specifications
├── scripts/                 # Test harnesses, clean-room validators, and multi-node failure demos
├── install.sh               # Interactive production installer script for Ubuntu / Debian
├── DEPLOYMENT.md            # Comprehensive production deployment manual
└── Makefile                 # Protobuf generation, compilation, and test automation
```

---

## 🛡️ 3. Fundamental Architectural Invariants & Rules

When modifying or extending Project Aurora, you **MUST** adhere to the following rules:

### A. Clean Architecture & DDD Isolation
1. **Domain Layer (`internal/domain/`)**: Pure business logic only. Never import database drivers, HTTP routers, gRPC packages, or third-party infrastructure frameworks into `internal/domain`.
2. **Authoritative Server-Side Authorization**: The backend remains strictly authoritative. Frontend UI checks are for UX only.
3. **Multi-Tenant Isolation**: Every customer mutation and query must enforce tenant boundary scoping (`tenant_id`). Customers must never access `/admin/*` or modify resources outside their tenant.

### B. Cryptographic Security & Audit Integrity
1. **Tamper-Evident SHA-256 Ledger**: Every security-sensitive or administrative mutation (node registration, key rotation, failover, billing mutation, role grant) **must** append a block to the cryptographic audit ledger (`internal/app/audit`).
2. **Secret Management**:
   - `AURORA_MASTER_KEY`: 256-bit AES-GCM hex key (32 bytes / 64 hex characters).
   - `AURORA_JWT_SECRET`: 512-bit HMAC-SHA256 hex secret (64 bytes / 128 hex characters).
   - Never log secrets, private keys, or passwords.
3. **Node mTLS Handshake**: Nodes enroll via single-use, expiring tokens and communicate exclusively over mutual TLS (`internal/infra/pki`).

### C. Financial & Metering Precision
1. **No Floating-Point Money**: Monetary values must always be stored and computed as integer minor units (e.g. cents / integer minor currency units).

### D. Concurrency & Race-Detector Compliance
1. All concurrent in-memory maps, caches, or background supervisors must be race-safe using `sync.RWMutex`, `sync.Mutex`, or atomic operations.
2. All code must pass `go test -race ./...`.

---

## ⚡ 4. Standard Commands & Developer Workflows

### Protobuf Generation
Protobuf compiler and plugin versions are strictly pinned for determinism:
- `protoc 29.3` (`v5.29.3`)
- `protoc-gen-go@v1.36.12`
- `protoc-gen-go-grpc@v1.5.1`

```bash
# Regenerate Go Protobuf bindings
make proto

# Verify zero uncommitted drift
git diff --exit-code gen/go
```

### Building Binaries
```bash
# Build both server and agent binaries into bin/
make build

# Build individual binaries
make build-server
make build-agent

# Build frontend SPA
make build-web
```

### Running Test Suites
```bash
# Run all Go unit & integration tests with race detector
go test -v -race ./...

# Run Frontend Vitest suite
cd web && npm run test

# Run TypeScript TypeCheck and build verification
cd web && npm run build

# Run clean-room deployment validator script
./scripts/audit-clean-room.sh

# Run end-to-end multi-node disaster recovery & self-healing demo
./scripts/demo.sh
```

### Local CI Validation with `act`
**Rule**: Always validate with `act` before committing/pushing CI changes:
```bash
# Run only Go backend checks
act -j backend-checks

# Run only React frontend checks
act -j frontend-checks

# Run full push workflow simulation
act push
```

---

## 🗄️ 5. Database & Migration Standards

1. Database migrations live in [`migrations/`](file:///home/lumi/Downloads/OVM%20V2/OVM%20V2/migrations) using versioned pairs:
   - `NNNNNN_description.up.sql`
   - `NNNNNN_description.down.sql`
2. Every migration must be **reversible** and **idempotent** (`IF NOT EXISTS`, `IF EXISTS`).
3. Control plane auto-migrates on startup when `AURORA_AUTO_MIGRATE=true`.

---

## 🌐 6. Ports & Protocols Reference

| Port | Protocol | Purpose | Access |
|---|---|---|---|
| `8080` | HTTP / WebSocket | REST API, Web Portal SPA, Metrics (`/metrics`), Probes (`/healthz`, `/readyz`) | Localhost / Reverse Proxy |
| `9443` | gRPC over mTLS | Distributed Hypervisor Node Agent Control Tunnel | Cluster Private / Firewall Allowed |
| `5432` | PostgreSQL | Control Plane State & Relational Store | Localhost / Private DB Subnet |
| `6379` | Redis / Valkey | Job Queue & Telemetry Cache (Optional) | Localhost / Private Subnet |
| `8443` | HTTPS / Incus | Incus Hypervisor Daemon Socket / API | Localhost / Hypervisor Internal |

---

## 📋 7. Agent Pre-Flight Checklist

Before proposing or finalizing any pull request or commit, verify:
- [ ] `make proto` produces 0 uncommitted diffs (`git diff --exit-code gen/go`).
- [ ] `go test -race ./...` passes 100% with 0 race warnings.
- [ ] `cd web && npm run test && npm run build` completes with 0 errors.
- [ ] `act push` passes all jobs successfully.
- [ ] Clean Architecture boundaries in `internal/domain` remain untouched by infrastructure dependencies.
- [ ] No secrets or credentials are hardcoded or leaked in logs.
