#!/usr/bin/env bash
set -euo pipefail

# 冒烟测试：真实启动服务、请求 /healthz、创建制品/版本/依赖、解析并校验版本集合，
# 最后以 trap 清理进程与临时数据。

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
PORT="${SMOKE_PORT:-18080}"
BASE_URL="http://127.0.0.1:${PORT}"
DB_DIR="$(mktemp -d)"
DB_FILE="${DB_DIR}/smoke.db"
LOG_FILE="$(mktemp)"
SERVER_PID=""

cleanup() {
  if [[ -n "${SERVER_PID}" ]] && kill -0 "${SERVER_PID}" 2>/dev/null; then
    kill "${SERVER_PID}" 2>/dev/null || true
    wait "${SERVER_PID}" 2>/dev/null || true
  fi
  rm -rf "${DB_DIR}" 2>/dev/null || true
  rm -f "${LOG_FILE}" /tmp/artifact-server /tmp/hz.json 2>/dev/null || true
}
trap cleanup EXIT

echo "building server..."
(cd "${ROOT_DIR}" && go build -o /tmp/artifact-server ./cmd/server)

echo "starting server on ${PORT}..."
LISTEN_ADDR=":${PORT}" DB_PATH="${DB_FILE}" /tmp/artifact-server >"${LOG_FILE}" 2>&1 &
SERVER_PID=$!

# 轮询健康检查。
ok=""
for _ in $(seq 1 50); do
  if curl -fsS "${BASE_URL}/healthz" >/tmp/hz.json 2>/dev/null; then
    ok="1"
    break
  fi
  sleep 0.2
done
if [[ -z "${ok}" ]]; then
  echo "FAIL: server did not become healthy" >&2
  cat "${LOG_FILE}" >&2
  exit 1
fi
echo "healthz: $(cat /tmp/hz.json)"

# 创建制品 a 与 b。
curl -fsS -X POST "${BASE_URL}/api/v1/artifacts" -H 'Content-Type: application/json' -d '{"name":"cli"}' >/dev/null
curl -fsS -X POST "${BASE_URL}/api/v1/artifacts" -H 'Content-Type: application/json' -d '{"name":"lib"}' >/dev/null

# 登记版本。
curl -fsS -X POST "${BASE_URL}/api/v1/artifacts/lib/versions" -H 'Content-Type: application/json' -d '{"version":"1.2.0"}' >/dev/null
curl -fsS -X POST "${BASE_URL}/api/v1/artifacts/lib/versions" -H 'Content-Type: application/json' -d '{"version":"1.3.0"}' >/dev/null
curl -fsS -X POST "${BASE_URL}/api/v1/artifacts/cli/versions" -H 'Content-Type: application/json' -d '{"version":"2.0.0"}' >/dev/null

# 声明依赖：cli@2.0.0 -> lib ^1.0.0。
curl -fsS -X PUT "${BASE_URL}/api/v1/artifacts/cli/versions/2.0.0/dependencies" \
  -H 'Content-Type: application/json' \
  -d '{"dependencies":[{"artifact":"lib","constraint":"^1.0.0"}]}' >/dev/null

# 解析。
RESOLVE_JSON="$(curl -fsS -X POST "${BASE_URL}/api/v1/resolve" -H 'Content-Type: application/json' -d '{"artifact":"cli"}')"
echo "resolve: ${RESOLVE_JSON}"

# 校验返回的版本集合包含 lib 的满足约束最高版本 1.3.0。
if ! echo "${RESOLVE_JSON}" | grep -q '"version":"1.3.0"'; then
  echo "FAIL: expected lib 1.3.0 in resolution" >&2
  exit 1
fi
if ! echo "${RESOLVE_JSON}" | grep -q '"status":"succeeded"'; then
  echo "FAIL: expected succeeded status" >&2
  exit 1
fi

# 提取 resolution_id，供后续扩写能力冒烟使用。
RESOLUTION_ID="$(echo "${RESOLVE_JSON}" | sed -n 's/.*"resolution_id":\([0-9]*\).*/\1/p')"
if [[ -z "${RESOLUTION_ID}" ]]; then
  echo "FAIL: missing resolution_id" >&2
  exit 1
fi

# 生成锁定快照并再次以锁定快照解析。
LOCK_JSON="$(curl -fsS -X POST "${BASE_URL}/api/v1/lockfiles" -H 'Content-Type: application/json' \
  -d "{\"resolution_id\":${RESOLUTION_ID},\"name\":\"smoke-lock\"}")"
echo "lockfile: ${LOCK_JSON}"
if ! echo "${LOCK_JSON}" | grep -q '"name":"smoke-lock"'; then
  echo "FAIL: lockfile not created" >&2
  exit 1
fi
if ! curl -fsS -X POST "${BASE_URL}/api/v1/resolve/lockfile" -H 'Content-Type: application/json' \
  -d '{"lockfile":"smoke-lock","artifact":"cli"}' | grep -q '"status":"in_sync"'; then
  echo "FAIL: lockfile resolve not in_sync" >&2
  exit 1
fi

# 依赖差异对比：为 cli 再登记一个版本并声明不同的约束，校验 changed 输出。
curl -fsS -X POST "${BASE_URL}/api/v1/artifacts/cli/versions" -H 'Content-Type: application/json' -d '{"version":"3.0.0"}' >/dev/null
curl -fsS -X PUT "${BASE_URL}/api/v1/artifacts/cli/versions/3.0.0/dependencies" \
  -H 'Content-Type: application/json' \
  -d '{"dependencies":[{"artifact":"lib","constraint":"^2.0.0"}]}' >/dev/null
DIFF_JSON="$(curl -fsS "${BASE_URL}/api/v1/artifacts/cli/versions/2.0.0/diff/3.0.0")"
echo "diff: ${DIFF_JSON}"
if ! echo "${DIFF_JSON}" | grep -q '"changed"'; then
  echo "FAIL: expected changed in diff" >&2
  exit 1
fi

# 发布就绪检查。
READY_JSON="$(curl -fsS "${BASE_URL}/api/v1/artifacts/cli/versions/2.0.0/readiness")"
echo "readiness: ${READY_JSON}"
if ! echo "${READY_JSON}" | grep -q '"ready":true'; then
  echo "FAIL: expected ready=true" >&2
  exit 1
fi

# 解析历史重跑。
if ! curl -fsS -X POST "${BASE_URL}/api/v1/resolutions/${RESOLUTION_ID}/rerun" | grep -q '"status":"succeeded"'; then
  echo "FAIL: rerun not succeeded" >&2
  exit 1
fi

echo "SMOKE TEST PASSED"
