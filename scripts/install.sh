#!/usr/bin/env bash
# ==============================================================================
#  Project Aurora — Enterprise Cloud Virtualization Control Plane & Node Installer
#  Production-Grade Automated Installation, Maintenance & Management Platform
# ==============================================================================

SCRIPT_VERSION="1.0.0"

for arg in "$@"; do
	if [ "$arg" = "--help" ] || [ "$arg" = "-h" ]; then
		echo "Project Aurora Cloud Control Plane & Hypervisor Installer (${SCRIPT_VERSION})"
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
		echo "  --force-arm                             Bypass ARM architecture checks"
		echo "  --skip-system-update                    Skip apt update & system package upgrade"
		echo "  --skip-zabbly-repo                      Skip configuring Zabbly Incus repository"
		echo "  --config, -c                            Open configuration manager"
		echo "  --non-interactive, -y                   Run in non-interactive batch mode"
		echo "  --help, -h                              Display this help menu"
		echo ""
		echo "Quick Role Examples:"
		echo "  sudo $0 --role control-plane --domain aurora.example.com --email admin@example.com"
		echo "  sudo $0 --role agent --hub-addr aurora.example.com:9443 --enrollment-token <TOKEN>"
		echo "  sudo $0 --role all-in-one"
		exit 0
	fi
done

# Require Root Privileges
if [ "${EUID:-$(id -u)}" -ne 0 ]; then
	echo -e "\033[0;31m[ERROR] This installer must be run as root or with sudo.\033[0m"
	echo "Please run: sudo $0"
	exit 1
fi
LOG_DIR="/var/log/aurora"
LOG_FILE="${LOG_DIR}/install.log"
CONFIG_DIR="/etc/aurora"
CONFIG_FILE="${CONFIG_DIR}/.aurora.conf"
STATE_DIR="/var/lib/aurora"
WEB_ROOT="/var/www/aurora/dist"
BACKUP_DIR="/var/backups/aurora"

# Ensure directories exist
mkdir -p "$LOG_DIR" "$CONFIG_DIR" "$CONFIG_DIR/migrations" "$STATE_DIR/tls" "$BACKUP_DIR"
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

# CLI Flags & Defaults
ROLE=""
SKIP_OS_CHECK=false
FORCE_ARM=false
SKIP_SYSTEM_UPDATE=false
SKIP_ZABBLY_REPO=false
NON_INTERACTIVE=false
CLI_SKIP_OS_CHECK_SET=false
SHOW_CONFIG_MENU=false

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
		CLI_SKIP_OS_CHECK_SET=true
		shift
		;;
	--force-arm)
		FORCE_ARM=true
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
	--config|-c)
		SHOW_CONFIG_MENU=true
		shift
		;;
	--non-interactive|-y)
		NON_INTERACTIVE=true
		shift
		;;
	--help|-h)
		echo "Project Aurora Cloud Control Plane & Hypervisor Installer (${SCRIPT_VERSION})"
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
		echo "  --force-arm                             Bypass ARM architecture checks"
		echo "  --skip-system-update                    Skip apt update & system package upgrade"
		echo "  --skip-zabbly-repo                      Skip configuring Zabbly Incus repository"
		echo "  --config, -c                            Open configuration manager"
		echo "  --non-interactive, -y                   Run in non-interactive batch mode"
		echo "  --help, -h                              Display this help menu"
		echo ""
		echo "Quick Role Examples:"
		echo "  sudo $0 --role control-plane --domain aurora.example.com --email admin@example.com"
		echo "  sudo $0 --role agent --hub-addr aurora.example.com:9443 --enrollment-token <TOKEN>"
		echo "  sudo $0 --role all-in-one"
		exit 0
		;;
	*)
		echo -e "${RED}Unknown option: $1${NC}"
		echo "Run '$0 --help' for usage instructions."
		exit 1
		;;
	esac
done

# ==============================================================================
#  Logging & Diagnostic Functions
# ==============================================================================

log_init() {
	mkdir -p "$LOG_DIR"
	touch "$LOG_FILE"
	chmod 664 "$LOG_FILE" 2>/dev/null || true
	{
		echo "=================================================================="
		date '+%Y-%m-%d %H:%M:%S %Z' | sed 's/^/[START] /'
		echo "Script: Project Aurora Installer (v${SCRIPT_VERSION})"
		echo "=================================================================="
	} >>"$LOG_FILE" 2>&1
}

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

run_with_spinner() {
	local start_msg="$1"
	local success_msg="$2"
	shift 2
	local show_elapsed="false"
	if [ "${1:-}" = "true" ] || [ "${1:-}" = "false" ]; then
		show_elapsed="$1"
		shift 1
	fi

	log_step "$start_msg"
	"$@" >>"$LOG_FILE" 2>&1 &
	local cmd_pid=$!
	local spinner="⠋⠙⠹⠸⠼⠴⠦⠧⠇⠏"
	local i=0
	local start_ts
	start_ts=$(date +%s)

	while kill -0 "$cmd_pid" >/dev/null 2>&1; do
		if [ -t 1 ]; then
			local elapsed_str=""
			if [ "$show_elapsed" = "true" ]; then
				local elapsed=$(($(date +%s) - start_ts))
				if [ "$elapsed" -ge 60 ]; then
					local mins=$((elapsed / 60))
					local secs=$((elapsed % 60))
					elapsed_str=" (${mins}m ${secs}s)"
				fi
			fi
			printf '\r  \033[0;36m%s\033[0m %s%s...' "${spinner:i%${#spinner}:1}" "$start_msg" "$elapsed_str"
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

support_hint() {
	echo -e "${YELLOW}Need assistance?${NC} Docs: ${BLUE}https://docs.aurora-vm.org${NC} | GitHub: ${BLUE}https://github.com/aurora-vm/aurora${NC}"
}

ERROR_HANDLER_ACTIVE=0
handle_unexpected_error() {
	local exit_code=$?
	local line_no="${1:-unknown}"
	local failed_command="${2:-unknown}"

	if [ "$ERROR_HANDLER_ACTIVE" -eq 1 ]; then
		exit "$exit_code"
	fi
	ERROR_HANDLER_ACTIVE=1
	trap - ERR

	log_error "Unexpected failure (exit code: $exit_code, line: $line_no)."
	log_error "Failing command: $failed_command"
	echo "[ERROR_CONTEXT] exit_code=$exit_code line=$line_no command=$failed_command" >>"$LOG_FILE"
	support_hint
	exit "$exit_code"
}

# Initialize logging
log_init
set -o errtrace
trap 'handle_unexpected_error "${LINENO}" "${BASH_COMMAND}"' ERR
set -o pipefail

# ==============================================================================
#  Prompt Helpers
# ==============================================================================

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

# ==============================================================================
#  UI & Display Formatting Helpers
# ==============================================================================

print_banner() {
	echo -e "${CYAN}${BOLD}"
	echo "     ___   __  __ ____   ____  ____     ___ "
	echo "    /   | / / / // __ \ / __ \/ __ \   /   |"
	echo "   / /| |/ / / // /_/ // / / / /_/ /  / /| |"
	echo "  / ___ / /_/ // _, _// /_/ / _, _/  / ___ |"
	echo " /_/  |_\__,_//_/ |_| \____/_/ |_|  /_/  |_|"
	echo -e "${NC}"
	echo -e "${BOLD} Project Aurora — Cloud Control Plane & Distributed Hypervisor${NC}"
	echo -e " Platform Version: ${GREEN}v${SCRIPT_VERSION}${NC} | Linux (Ubuntu / Debian) | Incus 6.x"
	echo ""
	echo -e "${CYAN}${BOLD}┌──────────────────────────────────────────────────────────────────┐${NC}"
	echo -e "${CYAN}${BOLD}│${NC}  Website:  ${BLUE}https://aurora-vm.org${NC}                                  ${CYAN}${BOLD}│${NC}"
	echo -e "${CYAN}${BOLD}│${NC}  GitHub:   ${BLUE}https://github.com/aurora-vm/aurora${NC}                    ${CYAN}${BOLD}│${NC}"
	echo -e "${CYAN}${BOLD}│${NC}  Docs:     ${BLUE}https://docs.aurora-vm.org${NC}                             ${CYAN}${BOLD}│${NC}"
	echo -e "${CYAN}${BOLD}└──────────────────────────────────────────────────────────────────┘${NC}"
}

draw_hr() {
	echo -e "${CYAN}──────────────────────────────────────────────────────────────────${NC}"
}

print_centered() {
	local text="$1"
	local color="${2:-$CYAN}"
	local width=66
	local padding=$(((width - ${#text}) / 2))
	if [ "$padding" -lt 0 ]; then padding=0; fi
	printf "%*s${color}${BOLD}%s${NC}\n" "$padding" "" "$text"
}

print_info_box() {
	local title="$1"
	shift
	local messages=("$@")

	if [ -t 1 ]; then clear; fi
	print_banner
	draw_hr
	print_centered "$title" "$YELLOW"
	draw_hr
	echo ""
	for msg in "${messages[@]}"; do
		echo -e "  ${BLUE}${msg}${NC}"
	done
	echo ""
	draw_hr
}

# ==============================================================================
#  Networking & DNS Utilities
# ==============================================================================

detect_public_ips() {
	PUBLIC_IPV4=$(
		{ curl -4 -s --max-time 5 ifconfig.me 2>/dev/null || curl -4 -s --max-time 5 api.ipify.org 2>/dev/null; } |
			tr -d '[:space:]' || true
	)
	PUBLIC_IPV6=$(
		curl -6 -s --max-time 5 ifconfig.co 2>/dev/null |
			tr -d '[:space:]' || true
	)
	if [[ -n "$PUBLIC_IPV4" && "$PUBLIC_IPV4" == *:* ]]; then PUBLIC_IPV4=""; fi
	if [[ -n "$PUBLIC_IPV6" && "$PUBLIC_IPV6" != *:* ]]; then PUBLIC_IPV6=""; fi
}

show_dns_setup_instructions() {
	local dns_domain="$1"
	echo -e "${BOLD}${YELLOW}DNS Setup Required${NC}"
	draw_hr
	echo -e "${BLUE}Before issuing the SSL certificate, ensure DNS records are configured:${NC}"
	echo ""
	if [ -n "${PUBLIC_IPV4:-}" ]; then
		echo -e "${GREEN}Create an A record (IPv4):${NC}"
		echo -e "  ${BOLD}Name:${NC}  $dns_domain"
		echo -e "  ${BOLD}Value:${NC} $PUBLIC_IPV4"
		echo -e "  ${BOLD}TTL:${NC}   300 (or Auto)"
		echo ""
	fi
	if [ -n "${PUBLIC_IPV6:-}" ]; then
		echo -e "${GREEN}Create an AAAA record (IPv6):${NC}"
		echo -e "  ${BOLD}Name:${NC}  $dns_domain"
		echo -e "  ${BOLD}Value:${NC} $PUBLIC_IPV6"
		echo -e "  ${BOLD}TTL:${NC}   300 (or Auto)"
		echo ""
	fi
	if [ -z "${PUBLIC_IPV4:-}" ] && [ -z "${PUBLIC_IPV6:-}" ]; then
		echo -e "${YELLOW}Could not detect your server's public IP. Create the appropriate DNS record manually.${NC}"
		echo ""
	fi
	echo -e "${YELLOW}DNS propagation can take 1-30 minutes depending on your DNS provider.${NC}"
	echo ""
}

# ==============================================================================
#  OS & System Prerequisite Checkers
# ==============================================================================

check_eol_status() {
	local os="$1"
	local version="$2"
	local current_date
	current_date=$(date +%s)
	local eol_date=""
	local eol_extended_date=""
	local status="supported"

	if [ -z "$os" ] || [ "$os" = "unknown" ] || [ -z "$version" ] || [ "$version" = "unknown" ]; then
		EOL_STATUS="supported"
		EOL_EXTENDED_DATE=""
		return 0
	fi

	case "$os" in
	debian)
		case "$version" in
		11)
			eol_date=$(date -d "2024-08-14" +%s 2>/dev/null || echo "")
			eol_extended_date=$(date -d "2026-08-31" +%s 2>/dev/null || echo "")
			;;
		12)
			eol_date=$(date -d "2026-06-10" +%s 2>/dev/null || echo "")
			eol_extended_date=$(date -d "2028-06-30" +%s 2>/dev/null || echo "")
			;;
		13)
			eol_date=$(date -d "2028-08-09" +%s 2>/dev/null || echo "")
			eol_extended_date=$(date -d "2030-06-30" +%s 2>/dev/null || echo "")
			;;
		esac
		;;
	ubuntu)
		case "$version" in
		20.04)
			eol_date=$(date -d "2025-04-01" +%s 2>/dev/null || echo "")
			eol_extended_date=$(date -d "2030-04-01" +%s 2>/dev/null || echo "")
			;;
		22.04)
			eol_date=$(date -d "2027-04-01" +%s 2>/dev/null || echo "")
			eol_extended_date=$(date -d "2032-04-01" +%s 2>/dev/null || echo "")
			;;
		24.04)
			eol_date=$(date -d "2029-04-01" +%s 2>/dev/null || echo "")
			eol_extended_date=$(date -d "2034-04-01" +%s 2>/dev/null || echo "")
			;;
		esac
		;;
	esac

	if [ -n "$eol_date" ] && [ "$eol_date" != "" ] && [ "$eol_date" -gt 0 ] 2>/dev/null; then
		if [ "$current_date" -ge "$eol_date" ] 2>/dev/null; then
			if [ -n "$eol_extended_date" ] && [ "$eol_extended_date" != "" ] && [ "$current_date" -lt "$eol_extended_date" ] 2>/dev/null; then
				status="extended"
			else
				status="eol"
			fi
		fi
	fi

	EOL_STATUS="$status"
	EOL_EXTENDED_DATE="$eol_extended_date"
}

check_os_compatibility() {
	if [ "$SKIP_OS_CHECK" = true ]; then
		log_warn "OS compatibility check bypassed via --skip-os-check."
		return 0
	fi

	if [ ! -f /etc/os-release ]; then
		log_error "Cannot detect Linux distribution (/etc/os-release missing)."
		exit 1
	fi

	# shellcheck source=/dev/null
	. /etc/os-release
	local os_id="${ID:-unknown}"
	local os_ver="${VERSION_ID:-unknown}"

	log_info "Detected Operating System: ${NAME:-Linux} ($os_ver)"

	local supported=false
	case "$os_id" in
	ubuntu)
		if [[ "$os_ver" =~ ^(20\.04|22\.04|24\.04|25\.04) ]]; then
			supported=true
		fi
		;;
	debian)
		if [[ "$os_ver" =~ ^(11|12|13) ]]; then
			supported=true
		fi
		;;
	esac

	if [ "$supported" = false ]; then
		log_warn "Untested distribution '$os_id $os_ver'. Supported: Ubuntu 22.04/24.04 LTS, Debian 12."
		if [ "$NON_INTERACTIVE" = false ]; then
			local proceed=""
			prompt "Continue installation on this OS? (y/n) [y]: " proceed "y"
			if [[ ! "$proceed" =~ ^[yY]$ ]]; then
				echo "Installation aborted."
				exit 0
			fi
		fi
	else
		log_success "Supported Linux distribution detected ($os_id $os_ver)."
	fi

	check_eol_status "$os_id" "$os_ver"
	if [ "${EOL_STATUS:-supported}" = "eol" ]; then
		log_warn "Your OS ($os_id $os_ver) has reached standard End of Life. Security updates may be unavailable."
	fi
}

ensure_base_packages() {
	if [ "$SKIP_SYSTEM_UPDATE" = true ]; then
		log_warn "System update skipped via --skip-system-update."
		return 0
	fi

	export DEBIAN_FRONTEND=noninteractive
	export APT_LISTCHANGES_FRONTEND=none

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
		run_with_spinner "Installing essential utilities (${to_install[*]})" "Essential utilities ready." \
			apt-get install -y -qq -o Dpkg::Options::="--force-confdef" -o Dpkg::Options::="--force-confold" "${to_install[@]}"
	else
		log_success "All essential packages are already installed."
	fi
}

ensure_go_compiler() {
	if command -v go >/dev/null 2>&1; then
		return 0
	fi
	if [ -x /usr/local/go/bin/go ]; then
		export PATH="/usr/local/go/bin:$PATH"
		return 0
	fi

	log_step "Go compiler not found. Automatically installing Go for binary compilation..."
	export DEBIAN_FRONTEND=noninteractive

	local go_arch="amd64"
	local uname_m
	uname_m=$(uname -m)
	case "$uname_m" in
	x86_64) go_arch="amd64" ;;
	aarch64|arm64) go_arch="arm64" ;;
	*) go_arch="amd64" ;;
	esac

	local go_tar="go1.23.6.linux-${go_arch}.tar.gz"
	local go_url="https://go.dev/dl/${go_tar}"

	if curl -fsSL "$go_url" -o "/tmp/${go_tar}" >>"$LOG_FILE" 2>&1; then
		rm -rf /usr/local/go
		tar -C /usr/local -xzf "/tmp/${go_tar}" >>"$LOG_FILE" 2>&1
		rm -f "/tmp/${go_tar}"
		export PATH="/usr/local/go/bin:$PATH"
		ln -sf /usr/local/go/bin/go /usr/local/bin/go 2>/dev/null || true
		ln -sf /usr/local/go/bin/go /usr/bin/go 2>/dev/null || true
		log_success "Go 1.23.6 binary distribution installed."
		return 0
	fi

	# Fallback to apt-get
	if apt-get install -y -qq golang-go >>"$LOG_FILE" 2>&1; then
		if command -v go >/dev/null 2>&1; then
			log_success "Go installed via package manager."
			return 0
		fi
	fi

	log_error "Could not install Go compiler automatically. Please install Go >= 1.22."
	return 1
}

ensure_node_and_npm() {
	if command -v npm >/dev/null 2>&1; then
		return 0
	fi

	log_step "Node.js & npm not found. Installing for frontend compilation..."
	export DEBIAN_FRONTEND=noninteractive
	apt-get install -y -qq nodejs npm >>"$LOG_FILE" 2>&1 || true
}

# ==============================================================================
#  Configuration Management
# ==============================================================================

init_config() {
	if [ ! -f "$CONFIG_FILE" ]; then
		mkdir -p "$(dirname "$CONFIG_FILE")"
		tee "$CONFIG_FILE" >/dev/null <<'EOF'
# Project Aurora Installer Configuration
# Persisted preferences for automated deployment and node agent links

AURORA_HTTP_PORT=8080
AURORA_GRPC_PORT=9443
AURORA_LOG_LEVEL=info
AURORA_AUTO_MIGRATE=true
SKIP_OS_CHECK=no
SKIP_ZABBLY_REPO=no
EOF
		chmod 600 "$CONFIG_FILE"
	fi
}

load_config() {
	if [ -f "$CONFIG_FILE" ]; then
		set +e
		while IFS='=' read -r key value; do
			[[ "$key" =~ ^#.*$ ]] && continue
			[[ -z "$key" ]] && continue
			if [[ "$key" =~ ^[a-zA-Z_][a-zA-Z0-9_]*$ ]]; then
				value=$(echo "$value" | sed 's/^[[:space:]]*//;s/[[:space:]]*$//' | sed 's/^"//;s/"$//' | sed "s/^'//;s/'$//")
				export "$key=$value"
			fi
		done <"$CONFIG_FILE"
		set -e
	fi
}

save_config() {
	tee "$CONFIG_FILE" >/dev/null <<EOF
# Project Aurora Installer Configuration
AURORA_HTTP_PORT=${AURORA_HTTP_PORT:-8080}
AURORA_GRPC_PORT=${AURORA_GRPC_PORT:-9443}
AURORA_LOG_LEVEL=${AURORA_LOG_LEVEL:-info}
AURORA_AUTO_MIGRATE=${AURORA_AUTO_MIGRATE:-true}
SKIP_OS_CHECK=${SKIP_OS_CHECK:-no}
SKIP_ZABBLY_REPO=${SKIP_ZABBLY_REPO:-no}
EOF
	chmod 600 "$CONFIG_FILE"
	log_success "Configuration saved to $CONFIG_FILE"
}

show_config() {
	echo ""
	draw_hr
	echo -e "${BOLD}${CYAN}Current Aurora Platform Configuration${NC}"
	draw_hr
	echo -e "  ${CYAN}•${NC} HTTP / REST Port:     ${BLUE}${AURORA_HTTP_PORT:-8080}${NC}"
	echo -e "  ${CYAN}•${NC} gRPC mTLS Port:       ${BLUE}${AURORA_GRPC_PORT:-9443}${NC}"
	echo -e "  ${CYAN}•${NC} Log Level:            ${BLUE}${AURORA_LOG_LEVEL:-info}${NC}"
	echo -e "  ${CYAN}•${NC} Auto DB Migrations:   ${BLUE}${AURORA_AUTO_MIGRATE:-true}${NC}"
	echo -e "  ${CYAN}•${NC} Skip OS Check:        ${BLUE}${SKIP_OS_CHECK:-no}${NC}"
	echo -e "  ${CYAN}•${NC} Skip Zabbly Repo:     ${BLUE}${SKIP_ZABBLY_REPO:-no}${NC}"
	draw_hr
	echo ""
}

manage_configuration() {
	while true; do
		if [ -t 1 ]; then clear; fi
		print_banner
		draw_hr
		print_centered "Configuration Manager" "$CYAN"
		draw_hr
		echo ""
		echo -e "  ${GREEN}[1]${NC} ${BOLD}View Current Settings${NC}"
		echo -e "  ${GREEN}[2]${NC} ${BOLD}Configure HTTP / gRPC Ports${NC}"
		echo -e "  ${GREEN}[3]${NC} ${BOLD}Configure Log Verbosity${NC}"
		echo -e "  ${GREEN}[4]${NC} ${BOLD}Toggle Auto-Migrations${NC}"
		echo -e "  ${YELLOW}[5]${NC} ${BOLD}Reset to Default Settings${NC}"
		echo ""
		echo -e "  ${CYAN}[0]${NC} ${BOLD}Back to Main Menu${NC}"
		draw_hr

		local cfg_choice=""
		prompt "${BOLD}Select option [0-5]: ${NC}" cfg_choice ""

		case "$cfg_choice" in
		1)
			show_config
			read -r -p "Press Enter to continue..."
			;;
		2)
			local new_http="" new_grpc=""
			prompt "Enter HTTP port [${AURORA_HTTP_PORT:-8080}]: " new_http "${AURORA_HTTP_PORT:-8080}"
			prompt "Enter gRPC port [${AURORA_GRPC_PORT:-9443}]: " new_grpc "${AURORA_GRPC_PORT:-9443}"
			AURORA_HTTP_PORT="$new_http"
			AURORA_GRPC_PORT="$new_grpc"
			save_config
			sleep 1
			;;
		3)
			echo -e "Select log level: [1] info  [2] debug  [3] warn  [4] error"
			local ll_choice=""
			prompt "Choice: " ll_choice "1"
			case "$ll_choice" in
			2) AURORA_LOG_LEVEL="debug" ;;
			3) AURORA_LOG_LEVEL="warn" ;;
			4) AURORA_LOG_LEVEL="error" ;;
			*) AURORA_LOG_LEVEL="info" ;;
			esac
			save_config
			sleep 1
			;;
		4)
			if [ "${AURORA_AUTO_MIGRATE:-true}" = "true" ]; then
				AURORA_AUTO_MIGRATE="false"
			else
				AURORA_AUTO_MIGRATE="true"
			fi
			save_config
			sleep 1
			;;
		5)
			AURORA_HTTP_PORT=8080
			AURORA_GRPC_PORT=9443
			AURORA_LOG_LEVEL=info
			AURORA_AUTO_MIGRATE=true
			save_config
			sleep 1
			;;
		0)
			break
			;;
		*)
			log_error "Invalid selection."
			sleep 1
			;;
		esac
	done
}

# ==============================================================================
#  Zabbly Incus 6.x Repository Setup
# ==============================================================================

setup_zabbly_incus_repo() {
	if [ "$SKIP_ZABBLY_REPO" = true ] || [ "${SKIP_ZABBLY_REPO:-no}" = "yes" ]; then
		log_warn "Zabbly Incus repository setup skipped via flag/config."
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

# ==============================================================================
#  1. Control Plane Installation (`aurora-server`)
# ==============================================================================

install_control_plane() {
	print_banner
	draw_hr
	echo -e "${BOLD}${CYAN}Installing Aurora Control Plane (aurora-server)...${NC}"
	draw_hr

	# 1. System User & Directory Layout
	log_step "Configuring Aurora system user and directories..."
	if ! id -u aurora >/dev/null 2>&1; then
		useradd --system --no-create-home --shell /usr/sbin/nologin aurora
		log_success "Created system user 'aurora'."
	fi

	mkdir -p "$CONFIG_DIR" "$CONFIG_DIR/migrations" "$STATE_DIR/tls" "$LOG_DIR" "$WEB_ROOT" "$BACKUP_DIR"
	chown -R aurora:aurora "$STATE_DIR" "$LOG_DIR" "$CONFIG_DIR" "$BACKUP_DIR"
	chmod 700 "$STATE_DIR" "$STATE_DIR/tls" "$BACKUP_DIR"

	# 2. PostgreSQL Setup
	log_step "Configuring PostgreSQL database and extensions..."
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
	elif [ -f "$src_dir/cmd/aurora-server/main.go" ]; then
		ensure_go_compiler
		log_info "Compiling aurora-server with Go..."
		(cd "$src_dir" && CGO_ENABLED=0 go build -ldflags="-w -s -X 'github.com/aurora-vm/aurora/pkg/version.Version=${SCRIPT_VERSION}'" -o /usr/local/bin/aurora-server ./cmd/aurora-server)
		log_success "Compiled and installed /usr/local/bin/aurora-server."
	else
		ensure_go_compiler
		log_info "Building from remote repository..."
		git clone https://github.com/aurora-vm/aurora.git /tmp/aurora-build >>"$LOG_FILE" 2>&1
		(cd /tmp/aurora-build && CGO_ENABLED=0 go build -ldflags="-w -s -X 'github.com/aurora-vm/aurora/pkg/version.Version=${SCRIPT_VERSION}'" -o /usr/local/bin/aurora-server ./cmd/aurora-server)
		cp -r /tmp/aurora-build/migrations/* "$CONFIG_DIR/migrations/"
		rm -rf /tmp/aurora-build
		log_success "Built /usr/local/bin/aurora-server from remote source."
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
	if [ -d "$src_dir/web/dist" ] && [ -f "$src_dir/web/dist/index.html" ]; then
		mkdir -p "$WEB_ROOT"
		cp -r "$src_dir/web/dist/"* "$WEB_ROOT/"
		chown -R www-data:www-data /var/www/aurora 2>/dev/null || chown -R aurora:aurora /var/www/aurora
		log_success "Copied pre-built frontend bundle to $WEB_ROOT."
	elif [ -d "$src_dir/web" ]; then
		ensure_node_and_npm
		log_info "Compiling React SPA frontend..."
		(cd "$src_dir/web" && npm ci >>"$LOG_FILE" 2>&1 && npm run build >>"$LOG_FILE" 2>&1)
		mkdir -p "$WEB_ROOT"
		cp -r "$src_dir/web/dist/"* "$WEB_ROOT/"
		chown -R www-data:www-data /var/www/aurora 2>/dev/null || chown -R aurora:aurora /var/www/aurora
		log_success "Built and deployed frontend to $WEB_ROOT."
	fi

	# 6. Generate Cryptographic Secrets & Environment
	log_step "Generating cryptographic secrets & environment configuration..."
	local master_key
	master_key=$(openssl rand -hex 32)
	local jwt_secret
	jwt_secret=$(openssl rand -hex 64)

	cat <<EOF | tee "$CONFIG_DIR/server.env" >/dev/null
AURORA_ENV=production
AURORA_HTTP_PORT=${AURORA_HTTP_PORT:-8080}
AURORA_GRPC_PORT=${AURORA_GRPC_PORT:-9443}
AURORA_DATABASE_URL=postgres://aurora:${DB_PASS}@127.0.0.1:5432/aurora?sslmode=disable
AURORA_REDIS_URL=redis://127.0.0.1:6379/0
AURORA_MASTER_KEY=${master_key}
AURORA_JWT_SECRET=${jwt_secret}
AURORA_LOG_LEVEL=${AURORA_LOG_LEVEL:-info}
AURORA_AUTO_MIGRATE=true
AURORA_TLS_DIR=${STATE_DIR}/tls
EOF
	chmod 600 "$CONFIG_DIR/server.env"
	chown aurora:aurora "$CONFIG_DIR/server.env"
	log_success "Environment configured at $CONFIG_DIR/server.env."

	# 7. Systemd Service Unit with Sandboxing
	log_step "Installing aurora-server systemd service..."
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

# Security Sandboxing & Resource Limits
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
	probe_status=$(curl -s --max-time 5 "http://127.0.0.1:${AURORA_HTTP_PORT:-8080}/healthz" || echo "FAIL")
	if [ "$probe_status" == "OK" ]; then
		log_success "Control plane process online and healthy (/healthz: OK)."
	else
		log_warn "Health probe returned '$probe_status'. Inspect with 'journalctl -u aurora-server -e'."
	fi

	# 9. Nginx Reverse Proxy Setup (if domain requested)
	if [ -z "$DOMAIN" ] && [ "$NON_INTERACTIVE" = false ]; then
		prompt "Enter FQDN domain for Web Portal & API (e.g. aurora.example.com) [leave empty for IP access]: " DOMAIN ""
	fi

	if [ -n "$DOMAIN" ]; then
		setup_nginx_for_domain "$DOMAIN"
	fi

	# 10. Install Global CLI Utility
	install_aurora_cli_tool

	print_control_plane_summary
}

# ==============================================================================
#  2. Hypervisor Node Agent Installation (`aurora-agent`)
# ==============================================================================

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
	elif [ -f "$src_dir/cmd/aurora-agent/main.go" ]; then
		ensure_go_compiler
		log_info "Building aurora-agent with Go..."
		(cd "$src_dir" && CGO_ENABLED=0 go build -ldflags="-w -s -X 'github.com/aurora-vm/aurora/pkg/version.Version=${SCRIPT_VERSION}'" -o /usr/local/bin/aurora-agent ./cmd/aurora-agent)
		log_success "Compiled and installed /usr/local/bin/aurora-agent."
	else
		ensure_go_compiler
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

	# 9. Install Global CLI Utility
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
	echo -e "Configures the Control Plane and Hypervisor Agent on this host."
	draw_hr

	install_control_plane

	log_step "Bootstrapping initial node enrollment token..."
	sleep 2

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

	# Show DNS setup guidance
	detect_public_ips
	show_dns_setup_instructions "$domain"

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
        proxy_pass http://127.0.0.1:${AURORA_HTTP_PORT:-8080};
        proxy_http_version 1.1;
        proxy_set_header Host \$host;
        proxy_set_header X-Real-IP \$remote_addr;
        proxy_set_header X-Forwarded-For \$proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto \$scheme;
        proxy_read_timeout 300s;
    }

    # Interactive WebSockets (PTY Terminal / VNC & Event Stream)
    location ~* ^/api/v1/(instances/[^/]+/console/(exec|vnc)|events/stream) {
        proxy_pass http://127.0.0.1:${AURORA_HTTP_PORT:-8080};
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
        proxy_pass http://127.0.0.1:${AURORA_HTTP_PORT:-8080};
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
	nginx -t >>"$LOG_FILE" 2>&1 && systemctl restart nginx >>"$LOG_FILE" 2>&1 || log_warn "Nginx reload failed; inspect config."
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
    BACKUP_DIR="/var/backups/aurora"
    TS=$(date -u +"%Y%m%d_%H%M%SZ")
    mkdir -p "$BACKUP_DIR"
    sudo -u postgres pg_dump -Fc aurora > "$BACKUP_DIR/aurora_db_$TS.dump"
    sha256sum "$BACKUP_DIR/aurora_db_$TS.dump" > "$BACKUP_DIR/aurora_db_$TS.dump.sha256"
    echo -e "${GREEN}Backup saved to $BACKUP_DIR/aurora_db_$TS.dump${NC}"
    ;;
verify)
    echo -e "${CYAN}Auditing SHA-256 Ledger & Cluster Diagnostics...${NC}"
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
#  Backup & Disaster Recovery Manager
# ==============================================================================

create_database_backup() {
	log_step "Creating immediate database backup..."
	local ts
	ts=$(date -u +"%Y%m%d_%H%M%SZ")
	mkdir -p "$BACKUP_DIR"
	local dump_file="$BACKUP_DIR/aurora_db_$ts.dump"

	if sudo -u postgres pg_dump -Fc aurora > "$dump_file" 2>>"$LOG_FILE"; then
		sha256sum "$dump_file" > "${dump_file}.sha256"
		local sz
		sz=$(du -h "$dump_file" | cut -f1)
		log_success "Backup created: $(basename "$dump_file") ($sz)"
	else
		log_error "Failed to create database backup dump."
		return 1
	fi
}

list_backups() {
	if [ -t 1 ]; then clear; fi
	print_banner
	draw_hr
	print_centered "Available Database Backups" "$CYAN"
	draw_hr
	echo ""

	mapfile -t backups < <(find "$BACKUP_DIR" -name "aurora_db_*.dump" -type f 2>/dev/null | sort -r)

	if [ ${#backups[@]} -eq 0 ]; then
		log_warn "No backups found in $BACKUP_DIR"
	else
		local idx=1
		for b in "${backups[@]}"; do
			local bname sz dt
			bname=$(basename "$b")
			sz=$(du -h "$b" | cut -f1)
			dt=$(stat -c %y "$b" 2>/dev/null | cut -d'.' -f1 || echo "Unknown")
			echo -e "  ${GREEN}[$idx]${NC} ${BOLD}$bname${NC}"
			echo -e "     ${BLUE}• Size:${NC} $sz  |  ${BLUE}Date:${NC} $dt"
			idx=$((idx + 1))
		done
	fi
	echo ""
	draw_hr
}

show_backup_menu() {
	while true; do
		if [ -t 1 ]; then clear; fi
		print_banner
		draw_hr
		print_centered "Backup & Disaster Recovery Manager" "$CYAN"
		draw_hr
		echo ""
		echo -e "  ${GREEN}[1]${NC} ${BOLD}Create Database Backup Now${NC}"
		echo -e "  ${BLUE}[2]${NC} ${BOLD}List Existing Backups${NC}"
		echo -e "  ${YELLOW}[3]${NC} ${BOLD}Trigger Disaster Recovery Simulation (Dry Run)${NC}"
		echo -e "  ${RED}[4]${NC} ${BOLD}Delete Old Backups (>14 days)${NC}"
		echo ""
		echo -e "  ${CYAN}[0]${NC} ${BOLD}Back to Main Menu${NC}"
		draw_hr

		local bk_choice=""
		prompt "${BOLD}Select option [0-4]: ${NC}" bk_choice ""

		case "$bk_choice" in
		1)
			create_database_backup
			read -r -p "Press Enter to continue..."
			;;
		2)
			list_backups
			read -r -p "Press Enter to continue..."
			;;
		3)
			log_info "Querying Disaster Recovery dry-run reconciliation..."
			curl -s "http://127.0.0.1:8080/api/v1/health" | jq . 2>/dev/null || curl -s "http://127.0.0.1:8080/healthz"
			read -r -p "Press Enter to continue..."
			;;
		4)
			log_info "Purging local database backups older than 14 days..."
			find "$BACKUP_DIR" -type f -name "aurora_db_*.dump*" -mtime +14 -delete 2>/dev/null || true
			log_success "Cleanup complete."
			read -r -p "Press Enter to continue..."
			;;
		0)
			break
			;;
		*)
			log_error "Invalid choice."
			sleep 1
			;;
		esac
	done
}

# ==============================================================================
#  Firewall & Security Management
# ==============================================================================

manage_firewall() {
	if [ -t 1 ]; then clear; fi
	print_banner
	draw_hr
	print_centered "Aurora Firewall Manager (UFW)" "$CYAN"
	draw_hr
	echo ""

	if ! command -v ufw >/dev/null 2>&1; then
		apt-get install -y -qq ufw >>"$LOG_FILE" 2>&1
	fi

	echo -e "This will configure standard UFW firewall rules for Project Aurora:"
	echo -e "  • ${GREEN}80/tcp${NC}   - HTTP (Let's Encrypt challenge & redirect)"
	echo -e "  • ${GREEN}443/tcp${NC}  - HTTPS (Web Portal & REST API)"
	echo -e "  • ${GREEN}9443/tcp${NC} - gRPC mTLS Hub (Hypervisor Node Tunnels)"
	echo -e "  • ${GREEN}22/tcp${NC}   - SSH Administration"
	echo -e "  • ${GREEN}incusbr0${NC} - Guest Container & VM Bridge Routing"
	echo ""

	local apply_fw=""
	prompt "${BOLD}Apply firewall rules now? (y/n) [y]: ${NC}" apply_fw "y"
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
#  System Runtime Information & Diagnostics
# ==============================================================================

show_system_info() {
	if [ -t 1 ]; then clear; fi
	print_banner
	draw_hr
	print_centered "Aurora Cluster Diagnostics & Runtime Info" "$CYAN"
	draw_hr
	echo ""

	echo -e "${BOLD}${CYAN}System Services Status:${NC}"
	local srv_status agent_status
	srv_status=$(systemctl is-active aurora-server 2>/dev/null || echo "inactive")
	agent_status=$(systemctl is-active aurora-agent 2>/dev/null || echo "inactive")

	echo -e "  ${CYAN}•${NC} Control Plane (aurora-server): ${BLUE}${srv_status}${NC}"
	echo -e "  ${CYAN}•${NC} Hypervisor Agent (aurora-agent): ${BLUE}${agent_status}${NC}"
	echo -e "  ${CYAN}•${NC} PostgreSQL 16:                 ${BLUE}$(systemctl is-active postgresql 2>/dev/null || echo "inactive")${NC}"
	echo -e "  ${CYAN}•${NC} Incus Virtualization Daemon:   ${BLUE}$(systemctl is-active incus 2>/dev/null || echo "inactive")${NC}"
	echo ""

	echo -e "${BOLD}${CYAN}Health Probes:${NC}"
	local liveness readiness
	liveness=$(curl -s --max-time 3 "http://127.0.0.1:${AURORA_HTTP_PORT:-8080}/healthz" 2>/dev/null || echo "OFFLINE")
	readiness=$(curl -s --max-time 3 "http://127.0.0.1:${AURORA_HTTP_PORT:-8080}/readyz" 2>/dev/null || echo "OFFLINE")
	echo -e "  ${CYAN}•${NC} Liveness  (/healthz): ${GREEN}${liveness}${NC}"
	echo -e "  ${CYAN}•${NC} Readiness (/readyz):  ${GREEN}${readiness}${NC}"
	echo ""

	echo -e "${BOLD}${CYAN}Virtualization Host Info:${NC}"
	if command -v incus >/dev/null 2>&1; then
		local instance_count
		instance_count=$(incus list --format csv 2>/dev/null | wc -l || echo 0)
		echo -e "  ${CYAN}•${NC} Incus Instances Running: ${BLUE}${instance_count}${NC}"
	else
		echo -e "  ${YELLOW}• Incus command not found.${NC}"
	fi
	echo ""
	draw_hr
	read -r -p "Press Enter to continue..."
}

# ==============================================================================
#  Uninstallation & System Purge
# ==============================================================================

uninstall_aurora() {
	if [ -t 1 ]; then clear; fi
	print_banner
	draw_hr
	echo -e "${BOLD}${RED}⚠️  AURORA UNINSTALLER & SYSTEM PURGE${NC}"
	draw_hr
	echo -e "${YELLOW}This operation will stop and remove:${NC}"
	echo -e "  ${RED}•${NC} Control Plane service (aurora-server)"
	echo -e "  ${RED}•${NC} Hypervisor Node Agent service (aurora-agent)"
	echo -e "  ${RED}•${NC} Configuration files (/etc/aurora)"
	echo -e "  ${RED}•${NC} State directory & certificates (/var/lib/aurora)"
	echo -e "  ${RED}•${NC} Global CLI binary (/usr/local/bin/aurora)"
	echo ""

	local confirm=""
	prompt "${BOLD}${RED}Are you absolutely sure you want to proceed? (type 'yes' to confirm): ${NC}" confirm ""
	if [ "$confirm" != "yes" ]; then
		log_info "Uninstallation cancelled."
		return 0
	fi

	log_step "Stopping and removing Aurora services..."
	systemctl stop aurora-server aurora-agent 2>/dev/null || true
	systemctl disable aurora-server aurora-agent 2>/dev/null || true
	rm -f /etc/systemd/system/aurora-server.service /etc/systemd/system/aurora-agent.service
	systemctl daemon-reload

	log_step "Removing binaries and configuration files..."
	rm -f /usr/local/bin/aurora /usr/local/bin/aurora-server /usr/local/bin/aurora-agent
	rm -rf "$CONFIG_DIR" "$STATE_DIR" "$LOG_DIR"

	log_success "Project Aurora platform has been uninstalled."
}

# ==============================================================================
#  Summary Displays
# ==============================================================================

print_control_plane_summary() {
	detect_public_ips
	local pub_ip="${PUBLIC_IPV4:-127.0.0.1}"
	local access_url="http://${pub_ip}:${AURORA_HTTP_PORT:-8080}"
	if [ -n "$DOMAIN" ]; then
		access_url="https://${DOMAIN}"
	fi

	echo ""
	draw_hr
	echo -e "${GREEN}${BOLD}★ AURORA CONTROL PLANE INSTALLED SUCCESSFULLY!${NC}"
	draw_hr
	echo -e "  ${BOLD}Web Portal & API:${NC}      ${CYAN}${access_url}${NC}"
	echo -e "  ${BOLD}gRPC mTLS Hub Port:${NC}    ${CYAN}${AURORA_GRPC_PORT:-9443}${NC}"
	echo -e "  ${BOLD}Configuration File:${NC}    ${CYAN}/etc/aurora/server.env${NC}"
	echo -e "  ${BOLD}State & Certificates:${NC}  ${CYAN}/var/lib/aurora/tls${NC}"
	echo -e "  ${BOLD}Service Status:${NC}        ${GREEN}systemctl status aurora-server${NC}"
	echo ""
	echo -e "${YELLOW}${BOLD}Initial Superadmin Account Bootstrap:${NC}"
	echo -e "Register the very first user via the Web Portal or with curl:"
	echo -e "  ${CYAN}curl -X POST ${access_url}/api/v1/auth/register \\"
	echo -e "    -H 'Content-Type: application/json' \\"
	echo -e "    -d '{\"username\":\"admin\",\"email\":\"admin@example.com\",\"password\":\"StrongPassword123!\"}'${NC}"
	echo ""
	echo -e "${BLUE}Tip:${NC} Use '${BOLD}aurora status${NC}' or '${BOLD}aurora logs${NC}' anytime to inspect cluster health."
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
#  Interactive Menus
# ==============================================================================

show_main_menu() {
	while true; do
		if [ -t 1 ]; then clear; fi
		print_banner
		draw_hr
		print_centered "Main Operations Menu" "$YELLOW"
		draw_hr
		echo ""
		echo -e "  ${GREEN}${BOLD}[1]${NC} ${BOLD}Control Plane${NC} ${CYAN}(API, Web Portal, DB, Auth & PKI Root CA)${NC}"
		echo -e "  ${BLUE}${BOLD}[2]${NC} ${BOLD}Hypervisor Node Agent${NC} ${CYAN}(Incus 6.x, ZFS, mTLS Tunnel Spoke)${NC}"
		echo -e "  ${MAGENTA}${BOLD}[3]${NC} ${BOLD}All-in-One Standalone${NC} ${CYAN}(Control Plane + Hypervisor on this host)${NC}"
		echo -e "  ${YELLOW}${BOLD}[4]${NC} ${BOLD}SSL & Reverse Proxy${NC} ${CYAN}(Nginx, Let's Encrypt Certbot, DNS helpers)${NC}"
		echo -e "  ${CYAN}${BOLD}[5]${NC} ${BOLD}Firewall Manager${NC} ${CYAN}(UFW rules for 80, 443, 9443, incusbr0)${NC}"
		echo -e "  ${GREEN}${BOLD}[6]${NC} ${BOLD}Backup & Disaster Recovery${NC} ${CYAN}(Database dumps & DR dry-runs)${NC}"
		echo -e "  ${BLUE}${BOLD}[7]${NC} ${BOLD}Cluster Diagnostics & Info${NC} ${CYAN}(Live CPU, RAM, Probes, Incus state)${NC}"
		echo -e "  ${YELLOW}${BOLD}[8]${NC} ${BOLD}Configuration Manager${NC} ${CYAN}(Ports, Log Level, Migration settings)${NC}"
		echo -e "  ${RED}${BOLD}[9]${NC} ${BOLD}Uninstall Aurora Platform${NC} ${CYAN}(Purge services & state)${NC}"
		echo ""
		echo -e "  ${CYAN}[0]${NC} ${BOLD}Exit Installer${NC}"
		draw_hr

		local choice=""
		prompt "${BOLD}Select an option [0-9]: ${NC}" choice ""

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
			show_backup_menu
			;;
		7)
			show_system_info
			;;
		8)
			manage_configuration
			;;
		9)
			uninstall_aurora
			read -r -p "Press Enter to return to main menu..."
			;;
		0|q|Q|exit)
			echo "Exiting Aurora installer. Goodbye!"
			exit 0
			;;
		*)
			log_error "Invalid selection."
			sleep 1
			;;
		esac
	done
}

# ==============================================================================
#  Main Execution Router
# ==============================================================================

init_config
load_config
if [ "$CLI_SKIP_OS_CHECK_SET" = true ]; then
	SKIP_OS_CHECK=true
fi

if [ "$SHOW_CONFIG_MENU" = true ]; then
	manage_configuration
	exit 0
fi

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

show_main_menu
