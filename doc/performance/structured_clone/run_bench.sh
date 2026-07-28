#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
repo_root="$(cd "${script_dir}/../../.." && pwd)"
base="${script_dir}/bench_run_"

(cd "${repo_root}" && go test -run=^$ -bench=^BenchmarkClone -benchmem -count=10 ./format/structured/.) > "${base}$(date +%Y-%m-%dT%H%M).txt"

# shellcheck disable=SC2046
benchstat $(for f in "${base}"*.txt; do printf '%s=%s ' "$(basename "$f" .txt | sed 's/^bench_run_//')" "$f"; done) | tee "${script_dir}/bench_summary.txt"
