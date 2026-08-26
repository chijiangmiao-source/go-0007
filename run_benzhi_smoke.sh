#!/usr/bin/env bash
set -euo pipefail

tmpdir="$(mktemp -d -p . .benzhi-smoke.XXXXXX)"
pid=""
cleanup() {
  if [[ -n "${pid}" ]] && kill -0 "${pid}" 2>/dev/null; then
    kill "${pid}" 2>/dev/null || true
    wait "${pid}" 2>/dev/null || true
  fi
  rm -rf "${tmpdir}"
}
trap cleanup EXIT

export GOCACHE="$(pwd)/${tmpdir}/gocache"
go build -o "${tmpdir}/leo-loop" ./cmd/leo-loop
"${tmpdir}/leo-loop" serve -addr 127.0.0.1:18080 -store "${tmpdir}/state.json" &
pid="$!"

for _ in $(seq 1 50); do
  if curl -fsS "http://127.0.0.1:18080/healthz" >/dev/null 2>&1; then
    break
  fi
  sleep 0.1
done

health="$(curl -fsS "http://127.0.0.1:18080/healthz")"
[[ "${health}" == *'"ok":true'* ]]

payload='{"station_id":"STA-ALPHA","arc_id":"SMOKE-ARC-001","confidence":0.93,"samples":[{"time":"2026-08-25T23:59:58.000Z","azimuth_deg":121.1,"elevation_deg":41.2},{"time":"2026-08-26T00:00:04.000Z","azimuth_deg":121.4,"elevation_deg":41.0},{"time":"2026-08-26T00:00:09.000Z","azimuth_deg":121.8,"elevation_deg":40.8}]}'
accepted="$(curl -fsS -H 'Content-Type: application/json' -d "${payload}" "http://127.0.0.1:18080/v1/observation-arcs")"
[[ "${accepted}" == *'"accepted":true'* ]]
[[ "${accepted}" == *'"associated_target_id":"TGT-000001"'* ]]

state="$(curl -fsS "http://127.0.0.1:18080/v1/system/state")"
[[ "${state}" == *'"arcs":1'* ]]
[[ "${state}" == *'"targets":1'* ]]

events="$(curl -fsS "http://127.0.0.1:18080/v1/system/events?limit=5")"
[[ "${events}" == *'observation.received'* ]]
[[ "${events}" == *'target.associated'* ]]

echo "benzhi smoke ok"
