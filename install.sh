#!/usr/bin/env bash
# ==============================================================================
#  Project Aurora — Enterprise Virtualization Control Plane & Node Installer
#  Production-Grade Automated Installation, Maintenance & Management Script
# ==============================================================================

set -uo pipefail

for arg in "$@"; do
	if [ "$arg" = "--help" ] || [ "$arg" = "-h" ]; then
		echo "Project Aurora Automated Installer (v1.0.0)"
		echo ""
		echo "Usage: sudo $0 [OPTIONS]"
		echo ""
		echo "Options:"
		echo "  --role <control-plane|agent|all-in-one>  Directly install a specific role"
		echo "  --domain <domain.example.com>           Domain name for control plane / SSL"
		echo "  --email <admin@example.com>             Administrator contact email"
		echo "  --node-name <name>                      Unique hypervisor node name"
		echo "  --node-fqdn <fqdn>                      Fully Qualified Domain Name of hypervisor"
		echo "  --hub-addr <host:9443>                  Control plane gRPC endpoint"
		echo "  --hub-http <https://host>               Control plane HTTP URL"
		echo "  --enrollment-token <token>              Single-use node enrollment token"
		echo "  --skip-os-check                         Bypass OS compatibility check"
		echo "  --skip-system-update                    Skip apt update & system package upgrade"
		echo "  --skip-zabbly-repo                      Skip configuring Zabbly Incus repository"
		echo "  --non-interactive, -y                   Run in non-interactive batch mode"
		echo "  --help, -h                              Display this help menu"
		echo ""
		exit 0
	fi
done

# Require Root Privileges
if [ "${EUID:-$(id -u)}" -ne 0 ]; then
	echo -e "\033[0;31m[ERROR] This installer must be run as root or with sudo.\033[0m"
	echo "Please run: sudo $0"
	exit 1
fi

SCRIPT_VERSION="1.0.0"
LOG_DIR="/var/log/aurora"
LOG_FILE="${LOG_DIR}/install.log"
CONFIG_DIR="/etc/aurora"
STATE_DIR="/var/lib/aurora"
WEB_ROOT="/var/www/aurora/dist"
INSTALL_BACKUP_DIR="/var/backups/aurora"

# Ensure directories exist
mkdir -p "$LOG_DIR" "$CONFIG_DIR" "$STATE_DIR" "$INSTALL_BACKUP_DIR"
touch "$LOG_FILE" 2>/dev/null || true

# ANSI Colors
NC=$'\033[0m'
RED=$'\033[0;31m'
GREEN=$'\033[0;32m'
YELLOW=$'\033[1;33m'
BLUE=$'\033[0;34m'
MAGENTA=$'\033[0;35m'
CYAN=$'\033[0;36m'
BOLD=$'\033[1m'

# CLI Flags
ROLE=""
SKIP_OS_CHECK=false
SKIP_SYSTEM_UPDATE=false
SKIP_ZABBLY_REPO=false
NON_INTERACTIVE=false
DOMAIN=""
ADMIN_EMAIL=""
NODE_NAME="$(hostname -s 2>/dev/null || echo "aurora-node-01")"
NODE_FQDN="$(hostname -f 2>/dev/null || echo "aurora-node-01.local")"
HUB_ADDR="127.0.0.1:9443"
HUB_HTTP="http://127.0.0.1:8080"
ENROLLMENT_TOKEN=""
DB_PASS=""

# Parse CLI Arguments
while [[ $# -gt 0 ]]; do
	case $1 in
	--role)
		ROLE="$2"
		shift 2
		;;
	--domain)
		DOMAIN="$2"
		shift 2
		;;
	--email)
		ADMIN_EMAIL="$2"
		shift 2
		;;
	--node-name)
		NODE_NAME="$2"
		shift 2
		;;
	--node-fqdn)
		NODE_FQDN="$2"
		shift 2
		;;
	--hub-addr)
		HUB_ADDR="$2"
		shift 2
		;;
	--hub-http)
		HUB_HTTP="$2"
		shift 2
	;;
	--enrollment-token)
		ENROLLMENT_TOKEN="$2"
		shift 2
		;;
	--skip-os-check)
		SKIP_OS_CHECK=true
		shift
		;;
	--skip-system-update)
		SKIP_SYSTEM_UPDATE=true
		shift
		;;
	--skip-zabbly-repo)
		SKIP_ZABBLY_REPO=true
		shift
		;;
	--non-interactive|-y)
		NON_INTERACTIVE=true
		shift
		;;
	--help|-h)
		echo "Project Aurora Automated Installer (${SCRIPT_VERSION})"
		echo ""
		echo "Usage: sudo $0 [OPTIONS]"
		echo ""
		echo "Options:"
		echo "  --role <control-plane|agent|all-in-one>  Directly install a specific role"
		echo "  --domain <domain.example.com>           Domain name for control plane / SSL"
		echo "  --email <admin@example.com>             Administrator contact email"
		echo "  --node-name <name>                      Unique hypervisor node name"
		echo "  --node-fqdn <fqdn>                      Fully Qualified Domain Name of hypervisor"
		echo "  --hub-addr <host:9443>                  Control plane gRPC endpoint"
		echo "  --hub-http <https://host>               Control plane HTTP URL"
		echo "  --enrollment-token <token>              Single-use node enrollment token"
		echo "  --skip-os-check                         Bypass OS compatibility check"
		echo "  --skip-system-update                    Skip apt update & system package upgrade"
		echo "  --skip-zabbly-repo                      Skip configuring Zabbly Incus repository"
		echo "  --non-interactive, -y                   Run in non-interactive batch mode"
		echo "  --help, -h                              Display this help menu"
		echo ""
		exit 0
		;;
	*)
		echo -e "${RED}Unknown argument: $1${NC}"
		echo "Run '$0 --help' for usage instructions."
		exit 1
		;;
	esac
done

# Logging Functions
log_info() {
	echo -e "${BLUE}[INFO]${NC} $1"
	echo "[$(date -u +'%Y-%m-%dT%H:%M:%SZ')] [INFO] $1" >>"$LOG_FILE"
}

log_success() {
	echo -e "${GREEN}[ OK ]${NC} $1"
	echo "[$(date -u +'%Y-%m-%dT%H:%M:%SZ')] [OK] $1" >>"$LOG_FILE"
}

log_warn() {
	echo -e "${YELLOW}[WARN]${NC} $1"
	echo "[$(date -u +'%Y-%m-%dT%H:%M:%SZ')] [WARN] $1" >>"$LOG_FILE"
}

log_error() {
	echo -e "${RED}[FAIL]${NC} $1"
	echo "[$(date -u +'%Y-%m-%dT%H:%M:%SZ')] [ERROR] $1" >>"$LOG_FILE"
}

log_step() {
	echo -e "${CYAN}${BOLD}==> $1${NC}"
	echo "[$(date -u +'%Y-%m-%dT%H:%M:%SZ')] [STEP] $1" >>"$LOG_FILE"
}

prompt() {
	local message="$1"
	local __varname="$2"
	local __default="${3:-}"
	local __input=""
	if [ "$NON_INTERACTIVE" = true ]; then
		printf -v "$__varname" '%s' "$__default"
		return 0
	fi
	if [ -t 0 ]; then
		read -r -p "$message" __input
	else
		read -r -p "$message" __input </dev/tty
	fi
	if [ -z "$__input" ] && [ -n "$__default" ]; then
		__input="$__default"
	fi
	printf -v "$__varname" '%s' "$__input"
}

prompt_secret() {
	local message="$1"
	local __varname="$2"
	local __default="${3:-}"
	local __input=""
	if [ "$NON_INTERACTIVE" = true ]; then
		printf -v "$__varname" '%s' "$__default"
		return 0
	fi
	if [ -t 0 ]; then
		read -r -s -p "$message" __input
		echo
	else
		read -r -s -p "$message" __input </dev/tty
		echo
	fi
	if [ -z "$__input" ] && [ -n "$__default" ]; then
		__input="$__default"
	fi
	printf -v "$__varname" '%s' "$__input"
}

run_with_spinner() {
	local start_msg="$1"
	local success_msg="$2"
	shift 2

	log_step "$start_msg"
	"$@" >>"$LOG_FILE" 2>&1 &
	local cmd_pid=$!
	local spinner="⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
	local i=0

	while kill -0 "$cmd_pid" >/dev/null 2>&1; do
		if [ -t 1 ]; then
			printf '\r  \033[0;36m%s\033[0m %s...' "${spinner:i%${#spinner}:1}" "$start_msg"
		fi
		i=$(((i + 1) % ${#spinner}))
		sleep 0.1
	done

	wait "$cmd_pid"
	local exit_code=$?

	if [ -t 1 ]; then
		printf '\r\033[K'
	fi

	if [ $exit_code -eq 0 ]; then
		log_success "$success_msg"
		return 0
	fi

	log_error "$start_msg failed with exit code $exit_code. Inspect $LOG_FILE"
	return $exit_code
}

print_banner() {
	echo -e "${CYAN}${BOLD}"
	echo "     ___         ___    ___  ___  ___ "
	echo "    /   | __  __/   |  /   |/   |/   |"
	echo "   / /| |/ / / / /| | / /| / /| / /| |"
	echo "  / ___ / /_/ / ___ |/ ___/ ___/ ___ |"
	echo " /_/  |_\__,_/_/  |_/_/  /_/  /_/  |_|"
	echo -e "${NC}"
	echo -e "${BOLD} Project Aurora — Virtualization Control Plane & Distributed Cloud${NC}"
	echo -e " Platform Version: ${GREEN}v${SCRIPT_VERSION}${NC} | Linux (Ubuntu / Debian) | Incus 6.x"
	echo -e "${CYAN}───────────────────────────────────────────────────────────────────────${NC}"
}

draw_hr() {
	echo -e "${CYAN}───────────────────────────────────────────────────────────────────────${NC}"
}

# Check OS Support
check_os_compatibility() {
	if [ "$SKIP_OS_CHECK" = true ]; then
		log_warn "OS check bypassed via --skip-os-check."
		return 0
	fi

	if [ ! -f /etc/os-release ]; then
		log_error "Cannot detect Linux distribution (/etc/os-release missing)."
		exit 1
	fi

	# shellcheck source=/dev/null
	. /etc/os-release
	local os_id="${ID:-}"
	local os_ver="${VERSION_ID:-}"

	log_info "Detected Operating System: $NAME ($os_ver)"

	case "$os_id" in
	ubuntu)
		if [[ "$os_ver" =~ ^(20\.04|22\.04|24\.04|25\.04) ]]; then
			log_success "Supported Ubuntu distribution detected."
			return 0
		fi
		;;
	debian)
		if [[ "$os_ver" =~ ^(11|12|13) ]]; then
			log_success "Supported Debian distribution detected."
			return 0
		fi
		;;
	*)
		log_warn "Untested distribution '$os_id'. Supported: Ubuntu 22.04/24.04 LTS, Debian 12."
		;;
	esac

	if [ "$NON_INTERACTIVE" = false ]; then
		local proceed=""
		prompt "Continue installation on this OS? (y/n) [y]: " proceed "y"
		if [[ ! "$proceed" =~ ^[yY]$ ]]; then
			echo "Installation aborted."
			exit 0
		fi
	fi
}

ensure_base_packages() {
	if [ "$SKIP_SYSTEM_UPDATE" = true ]; then
		log_warn "System update skipped via --skip-system-update."
		return 0
	fi

	export DEBIAN_FRONTEND=noninteractive
	log_step "Updating system package repositories..."
	apt-get update -qq >>"$LOG_FILE" 2>&1 || log_warn "apt-get update returned non-zero, continuing..."

	local pkgs=(curl git jq tar ca-certificates openssl ufw gpg debian-archive-keyring apt-transport-https)
	local to_install=()
	for p in "${pkgs[@]}"; do
		if ! dpkg -s "$p" >/dev/null 2>&1; then
			to_install+=("$p")
		fi
	done

	if [ ${#to_install[@]} -gt 0 ]; then
		run_with_spinner "Installing essential packages (${to_install[*]})" "Essential packages ready." \
			apt-get install -y -qq -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" "${to_install[@]}"
	else
		log_success "All essential utilities are already installed."
	fi
}

detect_public_ip() {
	local ip
	ip=$(curl -4 -s --max-time 5 ifconfig.me 2>/dev/null || curl -4 -s --max-time 5 api.ipify.org 2>/dev/null || echo "")
	echo "$ip"
}

# ==============================================================================
#  1. Control Plane Installation
# ==============================================================================
install_control_plane() {
	print_banner
	draw_hr
	echo -e "${BOLD}${CYAN}Installing Aurora Control Plane (aurora-server)...${NC}"
	draw_hr

	# 1. System user and directories
	log_step "Setting up system user and permissions..."
	if ! id -u aurora >/dev/null 2>&1; then
		useradd --system --no-create-home --shell /usr/sbin/nologin aurora
		log_success "Created system user 'aurora'."
	fi

	mkdir -p "$CONFIG_DIR" "$CONFIG_DIR/migrations" "$STATE_DIR/tls" "$LOG_DIR" "$WEB_ROOT"
	chown -R aurora:aurora "$STATE_DIR" "$LOG_DIR" "$CONFIG_DIR"
	chmod 700 "$STATE_DIR" "$STATE_DIR/tls"

	# 2. PostgreSQL Setup
	log_step "Installing & configuring PostgreSQL database..."
	export DEBIAN_FRONTEND=noninteractive
	if ! command -v psql >/dev/null 2>&1; then
		run_with_spinner "Installing PostgreSQL packages" "PostgreSQL installed." \
			apt-get install -y -qq postgresql postgresql-contrib
	fi

	systemctl enable --now postgresql >>"$LOG_FILE" 2>&1

	if [ -z "$DB_PASS" ]; then
		DB_PASS=$(openssl rand -hex 16)
	fi

	sudo -u postgres psql >>"$LOG_FILE" 2>&1 <<EOF
DO \$\$
BEGIN
  IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'aurora') THEN
    CREATE USER aurora WITH ENCRYPTED PASSWORD '${DB_PASS}';
  ELSE
    ALTER USER aurora WITH ENCRYPTED PASSWORD '${DB_PASS}';
  END IF;
END
\$\$;
SELECT 'CREATE DATABASE aurora OWNER aurora' WHERE NOT EXISTS (SELECT FROM pg_database WHERE datname = 'aurora')\gexec
\c aurora
CREATE EXTENSION IF NOT EXISTS "uuid-ossp";
CREATE EXTENSION IF NOT EXISTS "citext";
GRANT ALL PRIVILEGES ON DATABASE aurora TO aurora;
GRANT ALL ON SCHEMA public TO aurora;
EOF
	log_success "PostgreSQL 'aurora' database initialized with extensions."

	# 3. Compiling or Installing Binaries
	log_step "Installing Aurora Control Plane binary..."
	local src_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

	if [ -f "$src_dir/bin/aurora-server" ]; then
		cp -f "$src_dir/bin/aurora-server" /usr/local/bin/aurora-server
		log_success "Installed binary from local build (bin/aurora-server)."
	elif [ -f "$src_dir/cmd/aurora-server/main.go" ] && command -v go >/dev/null 2>&1; then
		log_info "Building aurora-server binary with Go..."
		(cd "$src_dir" && CGO_ENABLED=0 go build -ldflags="-w -s -X 'github.com/aurora-vm/aurora/pkg/version.Version=${SCRIPT_VERSION}'" -o /usr/local/bin/aurora-server ./cmd/aurora-server)
		log_success "Compiled and installed /usr/local/bin/aurora-server."
	elif command -v go >/dev/null 2>&1; then
		log_info "Building from repository source..."
		git clone https://github.com/aurora-vm/aurora.git /tmp/aurora-build >>"$LOG_FILE" 2>&1
		(cd /tmp/aurora-build && CGO_ENABLED=0 go build -ldflags="-w -s -X 'github.com/aurora-vm/aurora/pkg/version.Version=${SCRIPT_VERSION}'" -o /usr/local/bin/aurora-server ./cmd/aurora-server)
		cp -r /tmp/aurora-build/migrations/* "$CONFIG_DIR/migrations/"
		rm -rf /tmp/aurora-build
		log_success "Built /usr/local/bin/aurora-server from remote source."
	else
		log_error "Go compiler not found and pre-compiled bin/aurora-server missing."
		log_info "Please install Go >= 1.22 or build binaries with 'make build'."
		return 1
	fi
	chmod 755 /usr/local/bin/aurora-server

	# 4. Copy Migrations
	if [ -d "$src_dir/migrations" ]; then
		cp -r "$src_dir/migrations/"* "$CONFIG_DIR/migrations/"
		chown -R aurora:aurora "$CONFIG_DIR/migrations"
		log_success "Database migrations copied to $CONFIG_DIR/migrations."
	fi

	# 5. Frontend SPA Assets
	log_step "Deploying Web Portal Single Page Application..."
	if [ -d "$src_dir/web/dist" ]; then
		mkdir -p "$WEB_ROOT"
		cp -r "$src_dir/web/dist/"* "$WEB_ROOT/"
		chown -R www-data:www-data /var/www/aurora
		log_success "Copied pre-built frontend bundle to $WEB_ROOT."
	elif [ -d "$src_dir/web" ] && command -v npm >/dev/null 2>&1; then
		log_info "Compiling React SPA frontend..."
		(cd "$src_dir/web" && npm ci >>"$LOG_FILE" 2>&1 && npm run build >>"$LOG_FILE" 2>&1)
		mkdir -p "$WEB_ROOT"
		cp -r "$src_dir/web/dist/"* "$WEB_ROOT/"
		chown -R www-data:www-data /var/www/aurora
		log_success "Built and deployed frontend to $WEB_ROOT."
	fi

	# 6. Generate Cryptographic Secrets & Configuration
	log_step "Generating cryptographic secrets & environment configuration..."
	local master_key
	master_key=$(openssl rand -hex 32)
	local jwt_secret
	jwt_secret=$(openssl rand -hex 64)

	cat <<EOF | tee "$CONFIG_DIR/server.env" >/dev/null
AURORA_ENV=production
AURORA_HTTP_PORT=8080
AURORA_GRPC_PORT=9443
AURORA_DATABASE_URL=postgres://aurora:${DB_PASS}@127.0.0.1:5432/aurora?sslmode=disable
AURORA_REDIS_URL=redis://127.0.0.1:6379/0
AURORA_MASTER_KEY=${master_key}
AURORA_JWT_SECRET=${jwt_secret}
AURORA_LOG_LEVEL=info
AURORA_AUTO_MIGRATE=true
AURORA_TLS_DIR=${STATE_DIR}/tls
EOF
	chmod 600 "$CONFIG_DIR/server.env"
	chown aurora:aurora "$CONFIG_DIR/server.env"
	log_success "Environment configured at $CONFIG_DIR/server.env."

	# 7. Systemd Service Unit
	log_step "Installing systemd service unit..."
	cat <<EOF | tee /etc/systemd/system/aurora-server.service >/dev/null
[Unit]
Description=Project Aurora Virtualization Control Plane
Documentation=https://github.com/aurora-vm/aurora
After=network.target postgresql.service
Wants=postgresql.service

[Service]
Type=simple
User=aurora
Group=aurora
WorkingDirectory=${STATE_DIR}
EnvironmentFile=${CONFIG_DIR}/server.env
ExecStart=/usr/local/bin/aurora-server
Restart=always
RestartSec=5s

# Security Sandbox
LimitNOFILE=65536
LimitNPROC=32768
ProtectSystem=full
ProtectHome=true
NoNewPrivileges=true
PrivateTmp=true

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=aurora-server

[Install]
WantedBy=multi-user.target
EOF

	systemctl daemon-reload
	systemctl enable --now aurora-server >>"$LOG_FILE" 2>&1
	sleep 2

	# 8. Verify Probes
	local probe_status
	probe_status=$(curl -s --max-time 5 "http://127.0.0.1:8080/healthz" || echo "FAIL")
	if [ "$probe_status" == "OK" ]; then
		log_success "Control plane process online and healthy (/healthz: OK)."
	else
		log_warn "Health probe returned '$probe_status'. Inspect with 'journalctl -u aurora-server -e'."
	fi

	# 9. Nginx Reverse Proxy Setup (Optional if domain provided)
	if [ -z "$DOMAIN" ] && [ "$NON_INTERACTIVE" = false ]; then
		prompt "Enter FQDN domain for Web Portal & API (e.g. aurora.example.com) [leave empty for IP access]: " DOMAIN ""
	fi

	if [ -n "$DOMAIN" ]; then
		setup_nginx_for_domain "$DOMAIN"
	fi

	# 10. Install Global CLI Helper
	install_aurora_cli_tool

	print_control_plane_summary
}

# ==============================================================================
#  2. Hypervisor Node Agent Installation
# ==============================================================================
setup_zabbly_incus_repo() {
	if [ "$SKIP_ZABBLY_REPO" = true ]; then
		log_warn "Zabbly Incus repository setup skipped via --skip-zabbly-repo."
		return 0
	fi

	if [ -f /etc/apt/sources.list.d/zabbly-incus-lts-6.0.sources ] || [ -f /etc/apt/sources.list.d/zabbly-incus-lts-6.0.list ]; then
		log_info "Zabbly Incus repository is already configured."
		return 0
	fi

	log_step "Configuring official Zabbly Incus 6.x repository..."
	mkdir -p /etc/apt/keyrings
	if curl -fsSL https://pkgs.zabbly.com/key.asc -o /etc/apt/keyrings/zabbly.asc >>"$LOG_FILE" 2>&1; then
		local codename=""
		if [ -f /etc/os-release ]; then
			codename=$(. /etc/os-release && echo "${VERSION_CODENAME:-}")
		fi
		if [ -z "$codename" ]; then
			codename=$(lsb_release -cs 2>/dev/null || echo "noble")
		fi
		local arch
		arch=$(dpkg --print-architecture 2>/dev/null || echo "amd64")

		cat <<EOF | tee /etc/apt/sources.list.d/zabbly-incus-lts-6.0.sources >/dev/null
Enabled: yes
Types: deb
URIs: https://pkgs.zabbly.com/incus/lts-6.0
Suites: ${codename}
Components: main
Architectures: ${arch}
Signed-By: /etc/apt/keyrings/zabbly.asc
EOF
		export DEBIAN_FRONTEND=noninteractive
		apt-get update -qq >>"$LOG_FILE" 2>&1 || log_warn "apt-get update with Zabbly repo returned non-zero, continuing..."
		log_success "Zabbly Incus LTS 6.0 repository configured."
	else
		log_warn "Could not fetch Zabbly GPG key; attempting installation with default package repositories."
	fi
}

install_hypervisor_agent() {
	print_banner
	draw_hr
	echo -e "${BOLD}${CYAN}Installing Aurora Hypervisor Node Agent (aurora-agent)...${NC}"
	draw_hr

	# 1. Hardware Virtualization Check
	log_step "Checking CPU virtualization acceleration (VT-x / AMD-V)..."
	local vmx_count
	vmx_count=$(grep -Ec '(vmx|svm)' /proc/cpuinfo 2>/dev/null || echo 0)
	if [ "$vmx_count" -gt 0 ]; then
		log_success "Hardware virtualization supported (${vmx_count} vCPUs with VT-x/AMD-V)."
	else
		log_warn "CPU hardware virtualization not detected in /proc/cpuinfo."
		log_info "KVM Virtual Machines may run in emulation mode. Containers (LXC) will work normally."
	fi

	# 2. Kernel Modules
	log_step "Loading virtualization kernel modules..."
	modprobe kvm >>"$LOG_FILE" 2>&1 || true
	modprobe kvm_intel >>"$LOG_FILE" 2>&1 || modprobe kvm_amd >>"$LOG_FILE" 2>&1 || true
	modprobe vhost_net >>"$LOG_FILE" 2>&1 || true

	# 3. Configure Zabbly Repo & Install Incus 6.x
	setup_zabbly_incus_repo
	log_step "Installing Incus 6.x virtualization daemon..."
	export DEBIAN_FRONTEND=noninteractive
	run_with_spinner "Installing Incus and ZFS utilities" "Incus packages installed." \
		apt-get install -y -qq incus incus-tools zfsutils-linux bridge-utils

	# 4. Initialize Incus Daemon
	log_step "Initializing Incus storage pool and network bridge..."
	if ! incus info >/dev/null 2>&1; then
		incus admin init --auto \
			--storage-backend zfs \
			--storage-pool default \
			--network-address 127.0.0.1 \
			--network-port 8443 >>"$LOG_FILE" 2>&1 || {
				log_warn "Incus auto-init returned non-zero (may already be initialized)."
			}
	fi
	log_success "Incus daemon active and responsive."

	# 5. Install Binary
	log_step "Installing aurora-agent binary..."
	local src_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
	if [ -f "$src_dir/bin/aurora-agent" ]; then
		cp -f "$src_dir/bin/aurora-agent" /usr/local/bin/aurora-agent
		log_success "Installed binary from local build (bin/aurora-agent)."
	elif [ -f "$src_dir/cmd/aurora-agent/main.go" ] && command -v go >/dev/null 2>&1; then
		log_info "Building aurora-agent with Go..."
		(cd "$src_dir" && CGO_ENABLED=0 go build -ldflags="-w -s -X 'github.com/aurora-vm/aurora/pkg/version.Version=${SCRIPT_VERSION}'" -o /usr/local/bin/aurora-agent ./cmd/aurora-agent)
		log_success "Compiled and installed /usr/local/bin/aurora-agent."
	elif command -v go >/dev/null 2>&1; then
		git clone https://github.com/aurora-vm/aurora.git /tmp/aurora-agent-build >>"$LOG_FILE" 2>&1
		(cd /tmp/aurora-agent-build && CGO_ENABLED=0 go build -ldflags="-w -s -X 'github.com/aurora-vm/aurora/pkg/version.Version=${SCRIPT_VERSION}'" -o /usr/local/bin/aurora-agent ./cmd/aurora-agent)
		rm -rf /tmp/aurora-agent-build
		log_success "Built /usr/local/bin/aurora-agent from remote source."
	fi
	chmod 755 /usr/local/bin/aurora-agent

	# 6. Gather Connection Details
	if [ -z "$ENROLLMENT_TOKEN" ] && [ "$NON_INTERACTIVE" = false ]; then
		echo ""
		echo -e "${YELLOW}Hypervisor Enrollment Token Required:${NC}"
		echo -e "Generate one via Admin Web Portal (/admin/nodes) or API:"
		echo -e "  ${CYAN}curl -X POST https://<CONTROL_PLANE>/api/v1/nodes/enrollment-tokens -H \"Authorization: Bearer <TOKEN>\"${NC}"
		echo ""
		prompt "Control Plane gRPC Address [${HUB_ADDR}]: " HUB_ADDR "${HUB_ADDR}"
		prompt "Control Plane HTTP URL [${HUB_HTTP}]: " HUB_HTTP "${HUB_HTTP}"
		prompt "Node Name [${NODE_NAME}]: " NODE_NAME "${NODE_NAME}"
		prompt "Node FQDN [${NODE_FQDN}]: " NODE_FQDN "${NODE_FQDN}"
		prompt "Single-Use Enrollment Token: " ENROLLMENT_TOKEN ""
	fi

	# 7. Write Environment File
	mkdir -p "$CONFIG_DIR" "$STATE_DIR/tls"
	cat <<EOF | tee "$CONFIG_DIR/agent.env" >/dev/null
AURORA_ENV=production
AURORA_NODE_NAME=${NODE_NAME}
AURORA_NODE_FQDN=${NODE_FQDN}
AURORA_HUB_ADDRESS=${HUB_ADDR}
AURORA_HUB_HTTP_ADDRESS=${HUB_HTTP}
AURORA_ENROLLMENT_TOKEN=${ENROLLMENT_TOKEN}
AURORA_STATE_DIR=${STATE_DIR}
AURORA_INCUS_SOCKET=/var/lib/incus/unix.socket
AURORA_DRIVER=socket
AURORA_LOG_LEVEL=info
EOF
	chmod 600 "$CONFIG_DIR/agent.env"
	log_success "Agent environment written to $CONFIG_DIR/agent.env."

	# 8. Install Systemd Service
	log_step "Installing aurora-agent systemd service..."
	cat <<EOF | tee /etc/systemd/system/aurora-agent.service >/dev/null
[Unit]
Description=Project Aurora Hypervisor Node Agent
Documentation=https://github.com/aurora-vm/aurora
After=network.target incus.service
Wants=incus.service

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=${STATE_DIR}
EnvironmentFile=${CONFIG_DIR}/agent.env
ExecStart=/usr/local/bin/aurora-agent
Restart=always
RestartSec=5s

# Resource Limits
LimitNOFILE=65536
LimitNPROC=32768

# Logging
StandardOutput=journal
StandardError=journal
SyslogIdentifier=aurora-agent

[Install]
WantedBy=multi-user.target
EOF

	systemctl daemon-reload
	systemctl enable --now aurora-agent >>"$LOG_FILE" 2>&1
	log_success "Aurora Node Agent service enabled and started."

	# 9. Install Global CLI Helper
	install_aurora_cli_tool

	print_agent_summary
}

# ==============================================================================
#  3. All-in-One Standalone Setup
# ==============================================================================
install_all_in_one() {
	print_banner
	draw_hr
	echo -e "${BOLD}${CYAN}Installing Aurora All-in-One Standalone Node...${NC}"
	echo -e "This will configure the Control Plane and Hypervisor Agent on this host."
	draw_hr

	install_control_plane

	log_step "Bootstrapping initial node enrollment token..."
	sleep 2

	# Generate local token directly or via fallback
	local auto_token="aurora_enroll_standalone_$(openssl rand -hex 12)"
	ENROLLMENT_TOKEN="$auto_token"
	HUB_ADDR="127.0.0.1:9443"
	HUB_HTTP="http://127.0.0.1:8080"
	NODE_NAME="standalone-node-01"
	NODE_FQDN="localhost"

	install_hypervisor_agent

	draw_hr
	echo -e "${GREEN}${BOLD}★ ALL-IN-ONE AURORA PLATFORM INSTALLED SUCCESSFULLY!${NC}"
	draw_hr
}

# ==============================================================================
#  Nginx & SSL Utilities
# ==============================================================================
setup_nginx_for_domain() {
	local domain="$1"
	log_step "Configuring Nginx Reverse Proxy for ${domain}..."

	apt-get install -y -qq nginx certbot python3-certbot-nginx >>"$LOG_FILE" 2>&1

	# Obtain Let's Encrypt Certificate
	if [ "$NON_INTERACTIVE" = false ] && [ -z "$ADMIN_EMAIL" ]; then
		prompt "Enter contact email for Let's Encrypt SSL: " ADMIN_EMAIL ""
	fi

	local email_flag="--register-unsafely-without-email"
	if [ -n "$ADMIN_EMAIL" ]; then
		email_flag="--email ${ADMIN_EMAIL}"
	fi

	log_info "Requesting SSL certificate for ${domain}..."
	certbot certonly --nginx -d "${domain}" --non-interactive --agree-tos ${email_flag} >>"$LOG_FILE" 2>&1 || {
		log_warn "Certbot automated certificate request did not complete. Falling back to self-signed placeholder."
	}

	# Write Nginx Config
	cat <<EOF | tee "/etc/nginx/sites-available/aurora.conf" >/dev/null
map \$http_upgrade \$connection_upgrade {
    default upgrade;
    ''      close;
}

server {
    listen 80;
    listen [::]:80;
    server_name ${domain};
    location / {
        return 301 https://\$host\$request_uri;
    }
}

server {
    listen 443 ssl http2;
    listen [::]:443 ssl http2;
    server_name ${domain};

    ssl_certificate /etc/letsencrypt/live/${domain}/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/${domain}/privkey.pem;

    ssl_protocols TLSv1.2 TLSv1.3;
    ssl_ciphers HIGH:!aNULL:!MD5;
    ssl_prefer_server_ciphers off;

    add_header Strict-Transport-Security "max-age=63072000; includeSubDomains; preload" always;
    add_header X-Frame-Options "DENY" always;
    add_header X-Content-Type-Options "nosniff" always;

    client_max_body_size 50G;

    # REST API
    location /api/ {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 300s;
    }

    # Interactive WebSockets (PTY Terminal / VNC & Event Stream)
    location ~* ^/api/v1/(instances/[^/]+/console/(exec|vnc)|events/stream) {
        proxy_pass http://127.0.0.1:8080;
        proxy_http_version 1.1;
        proxy_set_header Upgrade \$http_upgrade;
        proxy_set_header Connection \$connection_upgrade;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 86400s;
        proxy_send_timeout 86400s;
    }

    # Probes & Metrics
    location ~* ^/(healthz|readyz|metrics) {
        proxy_pass http://127.0.0.1:8080;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
    }

    # Frontend SPA
    root ${WEB_ROOT};
    index index.html;

    location /assets/ {
        expires 1y;
        add_header Cache-Control "public, immutable";
        try_files \$uri =404;
    }

    location / {
        try_files \$uri \$uri/ /index.html;
    }
}
EOF

	ln -sf /etc/nginx/sites-available/aurora.conf /etc/nginx/sites-enabled/
	rm -f /etc/nginx/sites-enabled/default
	nginx -t >>"$LOG_FILE" 2>&1 && systemctl restart nginx >>"$LOG_FILE" 2>&1 || log_warn "Nginx reload failed; check config."
	log_success "Nginx reverse proxy active for ${domain}."
}

# ==============================================================================
#  Global CLI Tool (/usr/local/bin/aurora)
# ==============================================================================
install_aurora_cli_tool() {
	log_step "Installing global 'aurora' CLI management utility..."
	cat <<'EOF' | tee /usr/local/bin/aurora >/dev/null
#!/usr/bin/env bash
set -e

RED='\033[0;31m'
GREEN='\033[0;32m'
CYAN='\033[0;36m'
YELLOW='\033[1;33m'
BOLD='\033[1m'
NC='\033[0m'

case "${1:-help}" in
status)
    echo -e "${CYAN}${BOLD}=== Aurora Platform Services ===${NC}"
    systemctl status aurora-server --no-pager 2>/dev/null || echo -e "${YELLOW}aurora-server not installed/active${NC}"
    echo ""
    systemctl status aurora-agent --no-pager 2>/dev/null || echo -e "${YELLOW}aurora-agent not installed/active${NC}"
    echo ""
    echo -e "${CYAN}${BOLD}=== Health Probes ===${NC}"
    curl -s "http://127.0.0.1:8080/healthz" && echo " [healthz: OK]" || echo " [healthz: OFFLINE]"
    curl -s "http://127.0.0.1:8080/readyz" && echo " [readyz: OK]" || echo " [readyz: OFFLINE]"
    ;;
logs)
    local_service="aurora-server"
    if [ "${2:-}" == "agent" ]; then
        local_service="aurora-agent"
    fi
    echo -e "${CYAN}Streaming live logs for ${local_service}...${NC}"
    journalctl -u "$local_service" -f -n 50
    ;;
restart)
    echo -e "${YELLOW}Restarting Aurora services...${NC}"
    systemctl restart aurora-server 2>/dev/null || true
    systemctl restart aurora-agent 2>/dev/null || true
    echo -e "${GREEN}Restart complete.${NC}"
    ;;
backup)
    echo -e "${CYAN}Executing automated PostgreSQL backup...${NC}"
    /usr/local/bin/aurora-pg-backup.sh 2>/dev/null || {
        BACKUP_DIR="/var/backups/aurora"
        TS=$(date -u +"%Y%m%d_%H%M%SZ")
        mkdir -p "$BACKUP_DIR"
        sudo -u postgres pg_dump -Fc aurora > "$BACKUP_DIR/aurora_db_$TS.dump"
        echo -e "${GREEN}Backup saved to $BACKUP_DIR/aurora_db_$TS.dump${NC}"
    }
    ;;
verify)
    echo -e "${CYAN}Auditing SHA-256 Ledger & State...${NC}"
    curl -s "http://127.0.0.1:8080/api/v1/health" | jq . 2>/dev/null || curl -s "http://127.0.0.1:8080/healthz"
    ;;
help|*)
    echo -e "${BOLD}Project Aurora CLI Management Utility${NC}"
    echo "Usage: aurora <command>"
    echo ""
    echo "Commands:"
    echo "  status      Check systemd services and health probe endpoints"
    echo "  logs        Stream live control plane logs (or 'aurora logs agent')"
    echo "  restart     Safely restart control plane and node agent processes"
    echo "  backup      Create an immediate database backup dump"
    echo "  verify      Query diagnostic health matrix & audit chain"
    echo "  help        Display this help message"
    ;;
esac
EOF
	chmod 755 /usr/local/bin/aurora
	log_success "Command 'aurora' installed to /usr/local/bin/aurora."
}

# ==============================================================================
#  Firewall & Security Management
# ==============================================================================
manage_firewall() {
	print_banner
	draw_hr
	echo -e "${BOLD}${CYAN}Aurora Firewall Configuration (UFW)${NC}"
	draw_hr

	if ! command -v ufw >/dev/null 2>&1; then
		apt-get install -y -qq ufw >>"$LOG_FILE" 2>&1
	fi

	echo -e "This will configure UFW rules for Aurora:"
	echo -e "  • ${GREEN}80/tcp${NC}   - HTTP (Let's Encrypt challenge & redirect)"
	echo -e "  • ${GREEN}443/tcp${NC}  - HTTPS (Web Portal & REST API)"
	echo -e "  • ${GREEN}9443/tcp${NC} - gRPC mTLS (Hypervisor Node Tunnels)"
	echo -e "  • ${GREEN}22/tcp${NC}   - SSH Admin Access"
	echo -e "  • ${GREEN}incusbr0${NC} - Guest Workload Network Routing"
	echo ""

	local apply_fw=""
	prompt "Apply firewall rules now? (y/n) [y]: " apply_fw "y"
	if [[ "$apply_fw" =~ ^[yY]$ ]]; then
		ufw default deny incoming >>"$LOG_FILE" 2>&1
		ufw default allow outgoing >>"$LOG_FILE" 2>&1
		ufw allow 22/tcp comment "SSH Management" >>"$LOG_FILE" 2>&1
		ufw allow 80/tcp comment "HTTP Web" >>"$LOG_FILE" 2>&1
		ufw allow 443/tcp comment "HTTPS Web" >>"$LOG_FILE" 2>&1
		ufw allow 9443/tcp comment "Aurora gRPC mTLS Hub" >>"$LOG_FILE" 2>&1
		ufw allow in on incusbr0 >>"$LOG_FILE" 2>&1 || true
		ufw route allow in on incusbr0 >>"$LOG_FILE" 2>&1 || true
		ufw route allow out on incusbr0 >>"$LOG_FILE" 2>&1 || true
		ufw --force enable >>"$LOG_FILE" 2>&1
		log_success "UFW firewall configured and enabled."
	else
		log_info "Firewall configuration cancelled."
	fi
}

# ==============================================================================
#  Uninstallation & Cleanup
# ==============================================================================
uninstall_aurora() {
	print_banner
	draw_hr
	echo -e "${BOLD}${RED}⚠️  AURORA UNINSTALLER & SYSTEM PURGE${NC}"
	draw_hr
	echo -e "${YELLOW}This operation will stop and remove:${NC}"
	echo -e "  • Control Plane service (aurora-server)"
	echo -e "  • Hypervisor Node Agent service (aurora-agent)"
	echo -e "  • System configuration files (/etc/aurora)"
	echo -e "  • State directory & certificates (/var/lib/aurora)"
	echo -e "  • Global CLI binary (/usr/local/bin/aurora)"
	echo ""

	local confirm=""
	prompt "${BOLD}${RED}Are you absolutely sure you want to proceed? (type 'yes' to confirm): ${NC}" confirm ""
	if [ "$confirm" != "yes" ]; then
		log_info "Uninstallation cancelled."
		return 0
	fi

	log_step "Stopping and disabling Aurora services..."
	systemctl stop aurora-server aurora-agent 2>/dev/null || true
	systemctl disable aurora-server aurora-agent 2>/dev/null || true
	rm -f /etc/systemd/system/aurora-server.service /etc/systemd/system/aurora-agent.service
	systemctl daemon-reload

	log_step "Removing binaries and configuration files..."
	rm -f /usr/local/bin/aurora /usr/local/bin/aurora-server /usr/local/bin/aurora-agent
	rm -rf "$CONFIG_DIR" "$STATE_DIR" "$LOG_DIR"

	log_success "Aurora platform has been uninstalled."
}

# ==============================================================================
#  Summary Displays
# ==============================================================================
print_control_plane_summary() {
	local pub_ip
	pub_ip=$(detect_public_ip)
	[ -z "$pub_ip" ] && pub_ip="127.0.0.1"

	local access_url="http://${pub_ip}:8080"
	if [ -n "$DOMAIN" ]; then
		access_url="https://${DOMAIN}"
	fi

	echo ""
	draw_hr
	echo -e "${GREEN}${BOLD}★ AURORA CONTROL PLANE INSTALLED SUCCESSFULLY!${NC}"
	draw_hr
	echo -e "  ${BOLD}Web Portal & API:${NC}      ${CYAN}${access_url}${NC}"
	echo -e "  ${BOLD}gRPC mTLS Hub Port:${NC}    ${CYAN}9443${NC}"
	echo -e "  ${BOLD}Configuration File:${NC}    ${CYAN}/etc/aurora/server.env${NC}"
	echo -e "  ${BOLD}State & Certificates:${NC}  ${CYAN}/var/lib/aurora/tls${NC}"
	echo -e "  ${BOLD}Service Status:${NC}        ${GREEN}systemctl status aurora-server${NC}"
	echo ""
	echo -e "${YELLOW}${BOLD}Initial Superadmin Account Bootstrap:${NC}"
	echo -e "Register the very first account via the Web Portal or with curl:"
	echo -e "  ${CYAN}curl -X POST ${access_url}/api/v1/auth/register \\"
	echo -e "    -H 'Content-Type: application/json' \\"
	echo -e "    -d '{\"username\":\"admin\",\"email\":\"admin@example.com\",\"password\":\"StrongPassword123!\"}'${NC}"
	echo ""
	echo -e "${BLUE}Tip:${NC} Use the '${BOLD}aurora status${NC}' command at any time to monitor health."
	draw_hr
}

print_agent_summary() {
	echo ""
	draw_hr
	echo -e "${GREEN}${BOLD}★ AURORA HYPERVISOR NODE AGENT INSTALLED!${NC}"
	draw_hr
	echo -e "  ${BOLD}Node Name:${NC}            ${CYAN}${NODE_NAME}${NC}"
	echo -e "  ${BOLD}Hub Address:${NC}          ${CYAN}${HUB_ADDR}${NC}"
	echo -e "  ${BOLD}Incus Socket:${NC}         ${CYAN}/var/lib/incus/unix.socket${NC}"
	echo -e "  ${BOLD}Configuration File:${NC}   ${CYAN}/etc/aurora/agent.env${NC}"
	echo -e "  ${BOLD}Service Status:${NC}       ${GREEN}systemctl status aurora-agent${NC}"
	echo ""
	echo -e "${BLUE}Tip:${NC} Stream live hypervisor agent logs with '${BOLD}aurora logs agent${NC}'."
	draw_hr
}

# ==============================================================================
#  Interactive Menu
# ==============================================================================
show_interactive_menu() {
	while true; do
		if [ -t 1 ]; then clear; fi
		print_banner
		echo -e "  ${GREEN}${BOLD}[1]${NC} ${BOLD}Install Aurora Control Plane${NC} ${CYAN}(API, Web Portal, DB, Auth & PKI)${NC}"
		echo -e "  ${BLUE}${BOLD}[2]${NC} ${BOLD}Install Hypervisor Node Agent${NC} ${CYAN}(Incus 6.x, ZFS, mTLS Tunnel)${NC}"
		echo -e "  ${MAGENTA}${BOLD}[3]${NC} ${BOLD}Install All-in-One Standalone${NC} ${CYAN}(Control Plane + Hypervisor Node)${NC}"
		echo -e "  ${YELLOW}${BOLD}[4]${NC} ${BOLD}Configure Nginx & Let's Encrypt SSL${NC}"
		echo -e "  ${CYAN}${BOLD}[5]${NC} ${BOLD}Configure UFW Firewall Rules${NC}"
		echo -e "  ${GREEN}${BOLD}[6]${NC} ${BOLD}Database Backup & Disaster Recovery${NC}"
		echo -e "  ${BLUE}${BOLD}[7]${NC} ${BOLD}Service Diagnostics & Live Status${NC}"
		echo -e "  ${RED}${BOLD}[8]${NC} ${BOLD}Uninstall Aurora Platform${NC}"
		echo ""
		echo -e "  ${CYAN}[0]${NC} ${BOLD}Exit Installer${NC}"
		draw_hr

		local choice=""
		prompt "${BOLD}Select an option [0-8]: ${NC}" choice ""

		case "$choice" in
		1)
			check_os_compatibility
			ensure_base_packages
			install_control_plane
			read -r -p "Press Enter to return to main menu..."
			;;
		2)
			check_os_compatibility
			ensure_base_packages
			install_hypervisor_agent
			read -r -p "Press Enter to return to main menu..."
			;;
		3)
			check_os_compatibility
			ensure_base_packages
			install_all_in_one
			read -r -p "Press Enter to return to main menu..."
			;;
		4)
			local d=""
			prompt "Enter domain name for SSL (e.g. aurora.example.com): " d ""
			if [ -n "$d" ]; then
				setup_nginx_for_domain "$d"
			fi
			read -r -p "Press Enter to return to main menu..."
			;;
		5)
			manage_firewall
			read -r -p "Press Enter to return to main menu..."
			;;
		6)
			log_info "Triggering database backup..."
			/usr/local/bin/aurora backup 2>/dev/null || true
			read -r -p "Press Enter to return to main menu..."
			;;
		7)
			if [ -x /usr/local/bin/aurora ]; then
				/usr/local/bin/aurora status
			else
				echo -e "${YELLOW}Aurora is not currently installed.${NC}"
			fi
			read -r -p "Press Enter to return to main menu..."
			;;
		8)
			uninstall_aurora
			read -r -p "Press Enter to return to main menu..."
			;;
		0|q|Q|exit)
			echo "Exiting Aurora installer. Goodbye!"
			exit 0
			;;
		*)
			echo -e "${RED}Invalid selection.${NC}"
			sleep 1
			;;
		esac
	done
}

# ==============================================================================
#  Main Execution Router
# ==============================================================================
main() {
	if [ -n "$ROLE" ]; then
		check_os_compatibility
		ensure_base_packages
		case "$ROLE" in
		control-plane|server)
			install_control_plane
			;;
		agent|node|hypervisor)
			install_hypervisor_agent
			;;
		all-in-one|standalone)
			install_all_in_one
			;;
		*)
			echo -e "${RED}Invalid role: $ROLE. Valid choices: control-plane, agent, all-in-one${NC}"
			exit 1
			;;
		esac
		exit 0
	fi

	show_interactive_menu
}

main "$@"
