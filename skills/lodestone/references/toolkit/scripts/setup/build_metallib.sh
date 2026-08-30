#!/usr/bin/env bash
set -euo pipefail

configuration="${1:-release}"
if [[ "$configuration" != "release" && "$configuration" != "debug" ]]; then
  echo 'usage: build_metallib.sh [release|debug]' >&2
  exit 2
fi

toolkit_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
build_dir="${BUILD_DIR:-$toolkit_dir/runtime/.build}"
output_dir="$build_dir/$configuration"
kernels_dir="$build_dir/checkouts/mlx-swift/Source/Cmlx/mlx/mlx/backend/metal/kernels"
package_file="$build_dir/checkouts/mlx-swift/Package.swift"
output="$output_dir/mlx.metallib"

if [[ ! -d "$kernels_dir" || ! -d "$output_dir" || ! -f "$package_file" ]]; then
  echo 'MLX Swift build artifacts are missing; run swift build first.' >&2
  exit 1
fi
if [[ "${FORCE_METALLIB_BUILD:-0}" != "1" && -f "$output" && ! "${BASH_SOURCE[0]}" -nt "$output" &&
  -z "$(find "$kernels_dir" -type f \( -name '*.metal' -o -name '*.h' \) -newer "$output" -print -quit)" ]]; then
  exit 0
fi

sources=()
while IFS= read -r source; do
  sources+=("$source")
done < <(find "$kernels_dir" -type f -name '*.metal' ! -name '*_nax.metal' | LC_ALL=C sort)
if [[ "${#sources[@]}" -eq 0 ]]; then
  echo 'No MLX Metal sources were found.' >&2
  exit 1
fi

temporary="$(mktemp -d "${TMPDIR:-/tmp}/lodestone-metal.XXXXXX")"
trap 'rm -rf "$temporary"' EXIT

if ! xcrun --find metal >/dev/null 2>&1; then
  mlx_version="$(sed -n '/MLX_VERSION/s/.*\\"\([0-9][0-9]*\.[0-9][0-9]*\.[0-9][0-9]*\)\\".*/\1/p' "$package_file" | head -1)"
  if [[ -z "$mlx_version" ]]; then
    echo 'Could not determine the MLX core version.' >&2
    exit 1
  fi
  package_dir="$temporary/mlx-metal"
  UV_CACHE_DIR="${UV_CACHE_DIR:-${TMPDIR:-/tmp}/lodestone-uv-cache}" \
    uv pip install --target "$package_dir" --no-deps "mlx-metal==$mlx_version"
  cp "$package_dir/mlx/lib/mlx.metallib" "$output"
  exit 0
fi

air_files=()

for source in "${sources[@]}"; do
  relative="${source#"$kernels_dir/"}"
  key="$(printf '%s' "$relative" | shasum -a 256 | awk '{print $1}')"
  air="$temporary/$(printf '%s' "$key" | cut -c1-16).air"
  xcrun -sdk macosx metal -x metal -Wall -Wextra -fno-fast-math \
    -Wno-c++17-extensions -Wno-c++20-extensions \
    -c "$source" -I"$kernels_dir" \
    -I"$build_dir/checkouts/mlx-swift/Source/Cmlx/mlx" -o "$air"
  air_files+=("$air")
done

xcrun -sdk macosx metallib "${air_files[@]}" -o "$output"
