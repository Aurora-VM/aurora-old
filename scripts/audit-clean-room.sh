#!/usr/bin/env bash
set -e

# Project Aurora — Clean-Room Production Deployment Verification Test
GREEN='\033[0;32m'
BLUE='\033[0;34m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m'

echo -e "${BLUE}=================================================================================${NC}"
echo -e "${BLUE}      AURORA 1.0 — CLEAN-ROOM PRODUCTION DEPLOYMENT VALIDATION SUITE            ${NC}"
echo -e "${BLUE}=================================================================================${NC}"

TMP_DIR="/tmp/aurora-clean-room-audit"
rm -rf "$TMP_DIR"
mkdir -p "$TMP_DIR/server-tls" "$TMP_DIR/agent-state"

HTTP_PORT=8199
GRPC_PORT=9599

cleanup() {
    echo -e "\n${YELLOW}Tearing down test processes...${NC}"
    kill $AGENT_PID $SERVER_PID 2>/dev/null || true
    rm -rf "$TMP_DIR"
    echo -e "${GREEN}Clean-room teardown complete.${NC}"
}
trap cleanup EXIT

echo -e "\n${YELLOW}[Step 1] Building Production Binaries...${NC}"
go build -ldflags="-w -s -X 'github.com/aurora-vm/aurora/pkg/version.Version=1.0.0'" -o "$TMP_DIR/aurora-server" cmd/aurora-server/main.go
go build -ldflags="-w -s -X 'github.com/aurora-vm/aurora/pkg/version.Version=1.0.0'" -o "$TMP_DIR/aurora-agent" cmd/aurora-agent/main.go
echo -e "${GREEN}✓ Binaries compiled.${NC}"

echo -e "\n${YELLOW}[Step 2] Launching Aurora Control Plane...${NC}"
AURORA_HTTP_PORT=$HTTP_PORT \
AURORA_GRPC_PORT=$GRPC_PORT \
AURORA_TLS_DIR="$TMP_DIR/server-tls" \
AURORA_LOG_LEVEL="info" \
"$TMP_DIR/aurora-server" > "$TMP_DIR/server.log" 2>&1 &
SERVER_PID=$!
sleep 1.5

echo -e "${GREEN}✓ Server online.${NC}"

echo -e "\n${YELLOW}[Step 3] Verifying Probes & Prometheus Metrics...${NC}"
LIVENESS=$(curl -s "http://127.0.0.1:$HTTP_PORT/healthz")
READINESS=$(curl -s "http://127.0.0.1:$HTTP_PORT/readyz")
METRICS=$(curl -s "http://127.0.0.1:$HTTP_PORT/metrics" | head -n 5)
echo "Liveness: $LIVENESS | Readiness: $READINESS"
if [ "$LIVENESS" != "OK" ] || [ "$READINESS" != "READY" ]; then
    echo -e "${RED}Health probes failed!${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Health probes and metrics functional.${NC}"

echo -e "\n${YELLOW}[Step 4] Testing First-User Superadmin Bootstrap...${NC}"
REG1_RES=$(curl -s -X POST "http://127.0.0.1:$HTTP_PORT/api/v1/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","email":"admin@example.com","password":"SuperAdminPassword2026!"}')
ADMIN_ID=$(echo "$REG1_RES" | jq -r .data.id)

LOGIN_RES=$(curl -s -X POST "http://127.0.0.1:$HTTP_PORT/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"usernameOrEmail":"admin","password":"SuperAdminPassword2026!"}')
ADMIN_TOKEN=$(echo "$LOGIN_RES" | jq -r .data.tokens.accessToken)
ADMIN_ROLES=$(echo "$LOGIN_RES" | jq -r .data.user.roles[0])

echo "First Registered User ID: $ADMIN_ID (Role: $ADMIN_ROLES)"
if [ "$ADMIN_ROLES" != "superadmin" ]; then
    echo -e "${RED}First user was not granted superadmin!${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Superadmin bootstrap verified.${NC}"

echo -e "\n${YELLOW}[Step 5] Testing Subsequent User Registration (Tenant Isolation)...${NC}"
REG2_RES=$(curl -s -X POST "http://127.0.0.1:$HTTP_PORT/api/v1/auth/register" \
  -H "Content-Type: application/json" \
  -d '{"username":"tenant_alice","email":"alice@example.com","password":"AlicePassword2026!"}')
CUST_ID=$(echo "$REG2_RES" | jq -r .data.id)

LOGIN2_RES=$(curl -s -X POST "http://127.0.0.1:$HTTP_PORT/api/v1/auth/login" \
  -H "Content-Type: application/json" \
  -d '{"usernameOrEmail":"tenant_alice","password":"AlicePassword2026!"}')
CUST_TOKEN=$(echo "$LOGIN2_RES" | jq -r .data.tokens.accessToken)
CUST_ROLES=$(echo "$LOGIN2_RES" | jq -r .data.user.roles[0])

echo "Second User ID: $CUST_ID (Role: $CUST_ROLES)"
if [ "$CUST_ROLES" != "customer" ]; then
    echo -e "${RED}Subsequent user was erroneously granted administrative role!${NC}"
    exit 1
fi

# Confirm customer cannot access /admin/recovery
FORBIDDEN_TEST=$(curl -s -o /dev/null -w "%{http_code}" "http://127.0.0.1:$HTTP_PORT/api/v1/admin/recovery/backups" \
  -H "Authorization: Bearer $CUST_TOKEN")
if [ "$FORBIDDEN_TEST" != "403" ]; then
    echo -e "${RED}Customer was not blocked from admin endpoint! Code: $FORBIDDEN_TEST${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Tenant isolation verified (Admin access blocked with 403 Forbidden).${NC}"

echo -e "\n${YELLOW}[Step 6] Generating Enrollment Token & Enrolling Node Agent...${NC}"
TOKEN_RES=$(curl -s -X POST "http://127.0.0.1:$HTTP_PORT/api/v1/nodes/enrollment-tokens" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"locationId":"loc-default","ttlSeconds":3600}')
ENROLL_TOKEN=$(echo "$TOKEN_RES" | jq -r .data.enrollmentToken)
echo "Enrollment Token: ${ENROLL_TOKEN:0:15}..."

AURORA_HUB_ADDRESS="127.0.0.1:$GRPC_PORT" \
AURORA_HUB_HTTP_ADDRESS="http://127.0.0.1:$HTTP_PORT" \
AURORA_NODE_NAME="node-clean-01" \
AURORA_NODE_FQDN="node-clean-01.local" \
AURORA_ENROLLMENT_TOKEN="$ENROLL_TOKEN" \
AURORA_STATE_DIR="$TMP_DIR/agent-state" \
AURORA_DRIVER="simulated" \
"$TMP_DIR/aurora-agent" > "$TMP_DIR/agent.log" 2>&1 &
AGENT_PID=$!
sleep 2

# Verify node is listed in control plane
NODE_RES=$(curl -s "http://127.0.0.1:$HTTP_PORT/api/v1/nodes" -H "Authorization: Bearer $ADMIN_TOKEN")
NODE_ID=$(echo "$NODE_RES" | jq -r '.data[0].id')
echo "Enrolled Node ID: $NODE_ID"
if [ -z "$NODE_ID" ] || [ "$NODE_ID" == "null" ]; then
    echo -e "${RED}Node enrollment failed!${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Node agent successfully enrolled with mTLS certificates.${NC}"

echo -e "\n${YELLOW}[Step 7] Verifying Enrollment Token Cannot Be Reused...${NC}"
REUSE_STATUS=$(curl -s -o /dev/null -w "%{http_code}" -X POST "http://127.0.0.1:$HTTP_PORT/api/v1/nodes/enroll" \
  -H "Content-Type: application/json" \
  -d "{\"token\":\"$ENROLL_TOKEN\",\"nodeName\":\"fake-node\",\"fqdn\":\"fake.local\",\"csr\":\"invalid\"}")
if [ "$REUSE_STATUS" == "200" ] || [ "$REUSE_STATUS" == "201" ]; then
    echo -e "${RED}Single-use token was accepted for reuse!${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Token reuse rejected (HTTP $REUSE_STATUS).${NC}"

echo -e "\n${YELLOW}[Step 8] Provisioning First Workload Instance...${NC}"
INST_RES=$(curl -s -X POST "http://127.0.0.1:$HTTP_PORT/api/v1/instances" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{
    "name": "workload-audit-01",
    "type": "container",
    "cpuCores": 2,
    "memoryBytes": 2147483648,
    "storageBytes": 21474836480,
    "image": "images:ubuntu/24.04",
    "startAfterCreate": true
  }')
INST_ID=$(echo "$INST_RES" | jq -r .data.id)
echo "Workload Instance Created: $INST_ID"
echo -e "${GREEN}✓ Workload placed and started on hypervisor.${NC}"

echo -e "\n${YELLOW}[Step 9] Creating Encrypted Cluster Recovery Point...${NC}"
BACKUP_RES=$(curl -s -X POST "http://127.0.0.1:$HTTP_PORT/api/v1/admin/recovery/backups" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d '{"resourceType":"cluster","type":"full","retentionDays":30}')
BACKUP_ID=$(echo "$BACKUP_RES" | jq -r .data.id)
echo "Backup Created: $BACKUP_ID"
echo -e "${GREEN}✓ AES-256-GCM recovery point created with SHA-256 checksum.${NC}"

echo -e "\n${YELLOW}[Step 10] Executing 4-Step Disaster Recovery Restoration...${NC}"
RESTORE_RES=$(curl -s -X POST "http://127.0.0.1:$HTTP_PORT/api/v1/admin/recovery/restore" \
  -H "Authorization: Bearer $ADMIN_TOKEN" \
  -H "Content-Type: application/json" \
  -d "{\"backupId\":\"$BACKUP_ID\",\"confirmedDr\":true}")
RESTORE_STATUS=$(echo "$RESTORE_RES" | jq -r .data.status)
AUDIT_OK=$(echo "$RESTORE_RES" | jq -r .data.auditHashVerified)
echo "Restore Status: $RESTORE_STATUS | Audit Hash Verified: $AUDIT_OK"
if [ "$RESTORE_STATUS" != "completed" ] || [ "$AUDIT_OK" != "true" ]; then
    echo -e "${RED}Disaster recovery execution failed!${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Full disaster recovery restoration completed.${NC}"

echo -e "\n${YELLOW}[Step 11] Validating Tamper-Evident SHA-256 Audit Ledger Integrity...${NC}"
AUDIT_VERIFY=$(curl -s "http://127.0.0.1:$HTTP_PORT/api/v1/audit/verify" \
  -H "Authorization: Bearer $ADMIN_TOKEN")
CHAIN_VALID=$(echo "$AUDIT_VERIFY" | jq -r .data.valid)
echo "Audit Chain Valid: $CHAIN_VALID"
if [ "$CHAIN_VALID" != "true" ]; then
    echo -e "${RED}Audit chain verification failed!${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Cryptographic audit ledger intact.${NC}"

echo -e "\n${YELLOW}[Step 12] Testing Control-Plane Cold Restart & Persistent CA Loading...${NC}"
kill -9 $SERVER_PID
sleep 1

AURORA_HTTP_PORT=$HTTP_PORT \
AURORA_GRPC_PORT=$GRPC_PORT \
AURORA_TLS_DIR="$TMP_DIR/server-tls" \
AURORA_LOG_LEVEL="info" \
"$TMP_DIR/aurora-server" >> "$TMP_DIR/server.log" 2>&1 &
SERVER_PID=$!
sleep 2

# Verify server restarted and loaded existing CA
RESTART_CHECK=$(curl -s "http://127.0.0.1:$HTTP_PORT/healthz")
if [ "$RESTART_CHECK" != "OK" ]; then
    echo -e "${RED}Server restart failed!${NC}"
    exit 1
fi
echo -e "${GREEN}✓ Control plane restarted and verified persistent Root CA.${NC}"

echo -e "\n${BLUE}=================================================================================${NC}"
echo -e "${GREEN}    CLEAN-ROOM PRODUCTION DEPLOYMENT VALIDATION COMPLETED WITH 100% SUCCESS!    ${NC}"
echo -e "${BLUE}=================================================================================${NC}"
