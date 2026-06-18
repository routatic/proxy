#!/usr/bin/env bash
set -euo pipefail

# End-to-end test for model tool format and fallback fixes.
# Tests each model with a tool-containing request to verify:
#   - No "Unknown server-tool shorthand" 400 errors
#   - No temperature constraint violations
#   - All Go provider models work through the transform path
#
# Usage:
#   source .env && ./scripts/e2e-test.sh [--build]

REPO_ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$REPO_ROOT"

# --- Config ---
PORT="${OC_GO_CC_PORT:-3457}"
HOST="${OC_GO_CC_HOST:-127.0.0.1}"
BASE_URL="http://${HOST}:${PORT}"
TIMEOUT_SEC=60
pass=0
fail=0

# Colors
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
NC='\033[0m'

cleanup() {
	echo "=== Cleaning up ==="
	if [ -n "${PROXY_PID:-}" ]; then
		kill "${PROXY_PID}" 2>/dev/null || true
		wait "${PROXY_PID}" 2>/dev/null || true
	fi
	./bin/oc-go-cc stop 2>/dev/null || true
	sleep 1
	rm -f ~/.config/oc-go-cc/oc-go-cc.pid
}
trap cleanup EXIT

# --- Build ---
if [ "${1:-}" = "--skip-build" ]; then
	echo -e "${YELLOW}Skipping build...${NC}"
else
	echo "=== Building oc-go-cc ==="
	make build
	echo ""
fi

# --- Source .env (must be done BEFORE start) ---
if [ -f .env ]; then
	set -a
	# shellcheck disable=SC1091
	source .env
	set +a
fi

if [ -z "${OC_GO_CC_API_KEY:-}" ]; then
	echo -e "${RED}Error: OC_GO_CC_API_KEY not set. Create a .env file or export it.${NC}"
	exit 1
fi

# --- Start proxy ---
echo "=== Starting proxy on ${HOST}:${PORT} ==="
cleanup
# Run server in foreground but background it with & so we can capture the PID.
# Do NOT use -b (daemonize) because that forks and exits the parent, making $!
# capture the wrong PID.
./bin/oc-go-cc serve --port "$PORT" > /tmp/oc-go-cc-e2e.log 2>&1 &
PROXY_PID=$!
echo "Server PID: ${PROXY_PID}"

# Wait for health check with timeout
HEALTH_OK=false
for i in $(seq 1 10); do
	if curl -sf "${BASE_URL}/health" > /dev/null 2>&1; then
		HEALTH_OK=true
		break
	fi
	sleep 1
done
if [ "${HEALTH_OK}" != "true" ]; then
	echo -e "${RED}Proxy failed to start${NC}"
	cat /tmp/oc-go-cc-e2e.log 2>/dev/null || true
	exit 1
fi
echo -e "${GREEN}Proxy is running${NC}"
echo ""

# --- Test helper ---
test_model() {
	local model=$1
	local label=$2

	echo -n "  [$label] ${model} ... "

	REQUEST_BODY=$(cat <<'JSON'
{
	"model": "MODEL_PLACEHOLDER",
	"tools": [
		{
			"type": "custom",
			"name": "read_file",
			"description": "Read a file from the filesystem",
			"input_schema": {
				"type": "object",
				"properties": {
					"path": {"type": "string"}
				}
			}
		}
	],
	"messages": [
		{
			"role": "user",
			"content": [
				{"type": "text", "text": "Say hello and nothing else"}
			]
		}
	],
	"max_tokens": 100,
	"stream": false
}
JSON
)
	REQUEST_BODY="${REQUEST_BODY//MODEL_PLACEHOLDER/$model}"

	HTTP_CODE=$(curl -s -o /tmp/oc-go-cc-e2e-response.json -w '%{http_code}' \
		-X POST "${BASE_URL}/v1/messages" \
		-H "Content-Type: application/json" \
		-H "x-api-key: ${OC_GO_CC_API_KEY}" \
		-d "$REQUEST_BODY" \
		--max-time "$TIMEOUT_SEC")

	if [ "$HTTP_CODE" = 200 ]; then
		# Extract the text response for verification
		TEXT=$(python3 -c "
import json
with open('/tmp/oc-go-cc-e2e-response.json') as f:
    d = json.load(f)
blocks = d.get('content', [])
for b in blocks:
    if b.get('type') == 'text':
        print(b.get('text', ''))
" 2>/dev/null)
		echo -e "${GREEN}PASS${NC} (200, response: \"${TEXT}\")"
		pass=$((pass + 1))
	else
		ERROR_MSG=$(head -c 300 /tmp/oc-go-cc-e2e-response.json 2>/dev/null || echo "")
		echo -e "${RED}FAIL${NC} (HTTP ${HTTP_CODE})"
		echo "    Response: ${ERROR_MSG}"
		fail=$((fail + 1))
	fi
}

# --- Streaming test helper ---
test_streaming_model() {
	local model=$1
	local label=$2

	echo -n "  [${label}] ${model} (streaming) ... "

	REQUEST_BODY=$(cat <<'JSON'
{
	"model": "MODEL_PLACEHOLDER",
	"tools": [
		{
			"type": "custom",
			"name": "read_file",
			"description": "Read a file from the filesystem",
			"input_schema": {
				"type": "object",
				"properties": {
					"path": {"type": "string"}
				}
			}
		}
	],
	"messages": [
		{
			"role": "user",
			"content": [
				{"type": "text", "text": "Say hello and nothing else"}
			]
		}
	],
	"max_tokens": 100,
	"stream": true
}
JSON
)
	REQUEST_BODY="${REQUEST_BODY//MODEL_PLACEHOLDER/$model}"

	HTTP_CODE=$(curl -s -o /tmp/oc-go-cc-e2e-stream-response.txt -w '%{http_code}' \
		-X POST "${BASE_URL}/v1/messages" \
		-H "Content-Type: application/json" \
		-H "x-api-key: ${OC_GO_CC_API_KEY}" \
		-d "$REQUEST_BODY" \
		--max-time "$TIMEOUT_SEC")

	if [ "$HTTP_CODE" = 200 ]; then
		# Verify it's a valid SSE stream: must have message_start and message_stop
		if grep -q "event: message_start" /tmp/oc-go-cc-e2e-stream-response.txt && \
		   grep -q "event: message_stop" /tmp/oc-go-cc-e2e-stream-response.txt; then
			echo -e "${GREEN}PASS${NC} (200, valid SSE stream)"
			pass=$((pass + 1))
		else
			echo -e "${RED}FAIL${NC} (200 but missing message_start/message_stop — corrupted SSE)"
			head -c 400 /tmp/oc-go-cc-e2e-stream-response.txt
			fail=$((fail + 1))
		fi
	else
		ERROR_MSG=$(head -c 300 /tmp/oc-go-cc-e2e-stream-response.txt 2>/dev/null || echo "")
		echo -e "${RED}FAIL${NC} (HTTP ${HTTP_CODE})"
		echo "    Response: ${ERROR_MSG}"
		fail=$((fail + 1))
	fi
}

# --- Long streaming test helper (exercises heartbeat path) ---
test_streaming_long() {
	local model=$1
	local label=$2

	echo -n "  [${label}] ${model} (streaming long) ... "

	REQUEST_BODY=$(cat <<'JSON'
{
	"model": "MODEL_PLACEHOLDER",
	"messages": [
		{
			"role": "user",
			"content": [
				{"type": "text", "text": "Write a paragraph about the importance of testing in software engineering. Aim for 200 words."}
			]
		}
	],
	"max_tokens": 500,
	"stream": true
}
JSON
)
	REQUEST_BODY="${REQUEST_BODY//MODEL_PLACEHOLDER/$model}"

	HTTP_CODE=$(curl -s -o /tmp/oc-go-cc-e2e-stream-long.txt -w '%{http_code}' \
		-X POST "${BASE_URL}/v1/messages" \
		-H "Content-Type: application/json" \
		-H "x-api-key: ${OC_GO_CC_API_KEY}" \
		-d "$REQUEST_BODY" \
		--max-time 120)

	if [ "$HTTP_CODE" = 200 ]; then
		if grep -q "event: message_start" /tmp/oc-go-cc-e2e-stream-long.txt && \
		   grep -q "event: message_stop" /tmp/oc-go-cc-e2e-stream-long.txt; then
			DELTA_COUNT=$(grep -c "event: content_block_delta" /tmp/oc-go-cc-e2e-stream-long.txt 2>/dev/null || echo 0)
			echo -e "${GREEN}PASS${NC} (200, ${DELTA_COUNT} content deltas, valid SSE)"
			pass=$((pass + 1))
		else
			echo -e "${RED}FAIL${NC} (200 but invalid SSE — missing start/stop)"
			head -c 400 /tmp/oc-go-cc-e2e-stream-long.txt
			fail=$((fail + 1))
		fi
	else
		ERROR_MSG=$(head -c 300 /tmp/oc-go-cc-e2e-stream-long.txt 2>/dev/null || echo "")
		echo -e "${RED}FAIL${NC} (HTTP ${HTTP_CODE})"
		echo "    Response: ${ERROR_MSG}"
		fail=$((fail + 1))
	fi
}

# --- Test cases ---
echo "=== E2E Model Tests (with tools and custom type) ==="
echo ""

test_model "minimax-m3"       "Tools format fix (was 400)"
test_model "deepseek-v4-flash" "Baseline"
test_model "kimi-k2.7-code"   "Temperature fix (was 400)"
test_model "deepseek-v4-pro"  "Thinking model"
test_model "qwen3.7-plus"     "Go provider qwen (transform path)"
test_model "qwen3.7-max"      "Anthropic endpoint + sanitization"

echo ""
echo "=== E2E Streaming Tests (SSE proxying, heartbeat safety) ==="
echo ""

test_streaming_model "deepseek-v4-flash" "Streaming + tools"
test_streaming_model "deepseek-v4-pro"   "Streaming + thinking"
test_streaming_model "kimi-k2.7-code"    "Streaming Go provider"
test_streaming_model "minimax-m3"        "Streaming Anthropic endpoint"
test_streaming_long  "deepseek-v4-flash" "Long stream (heartbeat)"

echo ""
echo "=== Results ==="
echo -e "${GREEN}Passed: ${pass}${NC}"
echo -e "${RED}Failed: ${fail}${NC}"
echo ""

if [ "$fail" -gt 0 ]; then
	exit 1
fi
