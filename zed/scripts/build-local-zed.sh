#!/usr/bin/env bash
set -euo pipefail

script_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
extension_dir=$(cd "${script_dir}/.." && pwd)
build_dir="${extension_dir}/build"
grammar_dir="${extension_dir}/tree-sitter-bak"

if ! command -v cargo >/dev/null 2>&1; then
  echo "cargo is required; install Rust with rustup first" >&2
  exit 1
fi
if [[ -z "${WASI_SDK_PATH:-}" || ! -x "${WASI_SDK_PATH}/bin/clang" ]]; then
  echo "set WASI_SDK_PATH to a wasi-sdk installation (its bin/clang is required)" >&2
  exit 1
fi

mkdir -p "${build_dir}"
cargo build --release --target wasm32-wasip2 --target-dir "${build_dir}/target" --manifest-path "${extension_dir}/Cargo.toml"
cp "${build_dir}/target/wasm32-wasip2/release/bak_zed.wasm" "${build_dir}/extension.wasm"

if [[ ! -f "${grammar_dir}/src/parser.c" ]]; then
  echo "missing generated parser.c; run npm install && npm run generate in ${grammar_dir}" >&2
  exit 1
fi

"${WASI_SDK_PATH}/bin/clang" \
  -fPIC -shared -Os \
  -Wl,--export=tree_sitter_bak \
  -o "${build_dir}/bak.wasm" \
  -I "${grammar_dir}/src" \
  "${grammar_dir}/src/parser.c"

echo "Built ${build_dir}/extension.wasm and ${build_dir}/bak.wasm"
