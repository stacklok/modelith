#!/usr/bin/env bash
# mermaid-check.sh extracts every Mermaid diagram this repo ships and feeds it
# to the real Mermaid CLI's parser (@mermaid-js/mermaid-cli, "mmdc"), so a
# syntax error in generated or hand-written Mermaid is caught here instead of
# shipping a diagram that silently fails to render wherever it's viewed.
#
# Two source shapes are extracted, since the repo has both:
#   - ```mermaid fenced code blocks inside Markdown: examples/*.md and
#     docs/05-parking-garage/*.md (the renderer's committed golden output).
#   - raw, unfenced .mmd files: internal/render/mermaid/testdata/*.mmd (the
#     Go golden-test fixtures for the renderer, pinned as bare Mermaid source
#     with no surrounding fence). These are already exactly what mmdc expects,
#     so they're fed to it as-is — wrapping them in a throwaway fence just to
#     strip it back off before checking would be pure ceremony.
set -euo pipefail
shopt -s nullglob

root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$root"

workdir="$(mktemp -d)"
trap 'rm -rf "$workdir"' EXIT

fail=0
count=0

# check_file feeds one Mermaid source file to mmdc and reports failure with
# the offending label (a path, optionally with a block number) on parse error.
check_file() {
  local mmd="$1" label="$2"
  count=$((count + 1))
  local out="$workdir/check-$count.svg"
  local log="$workdir/check-$count.log"
  if ! npx -y @mermaid-js/mermaid-cli@11 -i "$mmd" -o "$out" >"$log" 2>&1; then
    echo "::error::mermaid-check: parse failed for $label"
    cat "$log"
    fail=1
  fi
}

# extract_and_check_md pulls every ```mermaid fenced block out of a Markdown
# file, writes each to its own temp file, and checks it. A line-oriented scan
# is enough here: modelith's own fences are exactly ```mermaid / ``` on their
# own line, and no source under examples/ or docs/05-parking-garage/ nests
# fences inside fences.
extract_and_check_md() {
  local md="$1"
  local rel="${md#"$root"/}"
  local block=0
  local in_block=0
  local out=""
  while IFS= read -r line || [ -n "$line" ]; do
    if [ "$in_block" -eq 0 ]; then
      if [ "$line" = '```mermaid' ]; then
        in_block=1
        block=$((block + 1))
        out="$workdir/extract-$(basename "$md")-$block.mmd"
        : >"$out"
      fi
      continue
    fi
    if [ "$line" = '```' ]; then
      in_block=0
      check_file "$out" "$rel block #$block"
      continue
    fi
    printf '%s\n' "$line" >>"$out"
  done <"$md"
}

md_files=(examples/*.md docs/05-parking-garage/*.md)
mmd_files=(internal/render/mermaid/testdata/*.mmd)

if [ ${#md_files[@]} -eq 0 ] && [ ${#mmd_files[@]} -eq 0 ]; then
  echo "::error::mermaid-check: no Mermaid sources found to check"
  exit 1
fi

for f in "${md_files[@]}"; do
  extract_and_check_md "$f"
done

for f in "${mmd_files[@]}"; do
  check_file "$f" "$f"
done

if [ "$fail" -ne 0 ]; then
  echo "mermaid-check: FAILED ($count block(s) checked)"
  exit 1
fi

echo "mermaid-check: OK ($count block(s) checked)"
