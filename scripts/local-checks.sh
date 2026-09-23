#!/usr/bin/env bash
# local-checks.sh — reproduce and document the behaviour of feat/unwrapped-tables.
#
# Usage:
#   ./scripts/local-checks.sh            # build dev + base, run every check
#   ./scripts/local-checks.sh tables     # table width/wrap matrix
#   ./scripts/local-checks.sh width      # terminal width detection + the 120 cap
#   ./scripts/local-checks.sh config     # config-file vs CLI-flag precedence
#   ./scripts/local-checks.sh search     # search highlight offset math (byte vs cell)
#
# Safety: nothing here touches ~/.config/glow/glow.yml. Every run gets its own
# throwaway HOME under /tmp/glow-local-checks.

set -uo pipefail

REPO="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
WORK="${TMPDIR:-/tmp}/glow-local-checks"
DEV_BIN="$WORK/glow-dev"
BASE_BIN="$WORK/glow-base"
BASE_REF="${BASE_REF:-7b2431d}"          # origin/main, before this branch
FIXTURE="$WORK/table.md"
PROSE="$WORK/prose.md"
W0_HOME="$WORK/home-width0"              # config: width: 0  (auto-detect)
CFG_HOME="$WORK/home-tablewrapfalse"     # config: table_wrap: false, table_width: 0

mkdir -p "$W0_HOME/.config/glow" "$CFG_HOME/.config/glow"

cat > "$FIXTURE" <<'MD'
# T

| A | B |
| --- | --- |
| aaaaaaaaaa bbbbbbbbbb cccccccccc dddddddddd eeeeeeeeee ffffffffff | 12345 67890 abcdefghij klmnopqrst uvwxyz0123 |
MD

cat > "$PROSE" <<'MD'
Lorem ipsum dolor sit amet consectetur adipiscing elit sed do eiusmod tempor incididunt ut labore et dolore magna aliqua ut enim ad minim veniam quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.
MD

cat > "$W0_HOME/.config/glow/glow.yml" <<'YML'
style: "dark"
width: 0
YML

cat > "$CFG_HOME/.config/glow/glow.yml" <<'YML'
style: "dark"
width: 0
table_wrap: false
table_width: 0
YML

# Widest line in *display* characters, after stripping ANSI. Byte counts are
# useless here: glamour's box-drawing characters are three bytes each, so a
# 38-cell line measures ~110 bytes.
maxlen() {
  sed -e 's/\x1b\[[0-9;?]*[a-zA-Z]//g' | awk '{ if (length($0) > m) m = length($0) } END { print m+0 }'
}
nlines() { wc -l | tr -d ' '; }

build() {
  echo "==> building branch binary"
  (cd "$REPO" && go build -o "$DEV_BIN" .) || { echo "FAIL: branch build"; exit 1; }
  if [[ ! -x "$BASE_BIN" ]]; then
    echo "==> building $BASE_REF binary (one-off; cached in $WORK)"
    local wt="$WORK/base-worktree"
    if git -C "$REPO" worktree add "$wt" "$BASE_REF" >/dev/null 2>&1; then
      (cd "$wt" && go build -o "$BASE_BIN" .) || echo "SKIP: base build failed"
      git -C "$REPO" worktree remove --force "$wt" >/dev/null 2>&1
    else
      echo "SKIP: could not create worktree for $BASE_REF"
    fi
  fi
}

tables() {
  echo
  echo "### 1. Table width / wrap matrix  (fixture: two very wide cells, --width 40)"
  printf '%-56s %-8s %s\n' "invocation" "maxcells" "lines"
  for args in "--width 40" "--width 40 --table-wrap=true" "--width 40 --table-wrap=false" \
              "--width 40 --table-width 60" "--width 40 --table-wrap=true --table-width 60" \
              "--width 40 --table-wrap=false --table-width 0"; do
    out=$(HOME="$W0_HOME" "$DEV_BIN" -p $args "$FIXTURE" 2>/dev/null)
    printf 'dev  %-51s %-8s %s\n' "$args" "$(echo "$out" | maxlen)" "$(echo "$out" | nlines)"
  done
  [[ -x "$BASE_BIN" ]] && {
    out=$(HOME="$W0_HOME" "$BASE_BIN" -p --width 40 "$FIXTURE" 2>/dev/null)
    printf 'base %-51s %-8s %s\n' "--width 40 ($BASE_REF)" "$(echo "$out" | maxlen)" "$(echo "$out" | nlines)"
  }
  echo
  echo "Expected now: the default and --table-wrap=true wrap cells to the render"
  echo "width (same as base), --table-wrap=false renders at natural content width,"
  echo "and --table-width N lays the table out at N columns."
}

width() {
  echo
  echo "### 2. Terminal width detection (config width: 0, stdout piped)"
  for c in 80 150 200; do
    d=$(COLUMNS=$c HOME="$W0_HOME" "$DEV_BIN" "$PROSE" 2>/dev/null | maxlen)
    b=$(COLUMNS=$c HOME="$W0_HOME" ${BASE_BIN:-/bin/true} "$PROSE" 2>/dev/null | maxlen)
    printf 'COLUMNS=%-4s base maxlen=%-4s dev maxlen=%s\n' "$c" "$b" "$d"
  done
  echo
  echo "Read: the branch honours \$COLUMNS (base never did) and no longer caps at 120,"
  echo "so prose spans the whole terminal on wide displays. => known-issues.md #5"
  echo
  echo "Gotcha: if no glow.yml exists, glow WRITES one with 'width: 80' on first run,"
  echo "which silently disables auto-detection afterwards. That is why a pristine"
  echo "HOME looks like COLUMNS is ignored; always test with an explicit width: 0 config."
}

config() {
  echo
  echo "### 3. Config file vs CLI flags (config sets table_wrap: false, table_width: 0)"
  for args in "" "--table-wrap=true" "--table-wrap=false" "--table-width 60"; do
    out=$(HOME="$CFG_HOME" "$DEV_BIN" -p --width 40 $args "$FIXTURE" 2>/dev/null)
    printf 'dev + config(table_wrap:false) %-30s maxcells=%-6s lines=%s\n' \
      "${args:-<no flags>}" "$(echo "$out" | maxlen)" "$(echo "$out" | nlines)"
  done
  echo
  echo "### 3b. Environment (no table keys in the config)"
  for env in "GLOW_TABLE_WRAP=false" "GLOW_TABLE_WRAP=true" "GLOW_TABLE_WIDTH=60"; do
    out=$(env HOME="$W0_HOME" $env "$DEV_BIN" -p --width 40 "$FIXTURE" 2>/dev/null)
    printf 'dev + %-28s maxcells=%-6s lines=%s\n' "$env" "$(echo "$out" | maxlen)" "$(echo "$out" | nlines)"
  done
  echo
  echo "Expected now: with the config present, --table-wrap=true still wins (cells"
  echo "wrap, ~38). With no flags the config applies (natural width, ~116)."
  echo "GLOW_TABLE_WRAP=false behaves like --table-wrap=false."
}

search() {
  echo
  echo "### 4. Search highlight offsets — exercises the real findMatches()/applyHighlights()"
  (cd "$REPO" && go test ./ui -v \
      -run 'TestFindMatchesUsesCellOffsets|TestApplyHighlightsSelectsCurrentMatchByIndex|TestFirstMatchInView|TestSearchSurvivesReRender' 2>&1 | tail -30)
  echo
  echo "These cover: cell (not byte/rune) offsets for box-drawing, accents, CJK and"
  echo "emoji; the current match being selected by index so duplicates are not all"
  echo "marked; and matches staying put across a re-render."
}

case "${1:-all}" in
  tables) build; tables ;;
  width)  build; width ;;
  config) build; config ;;
  search) search ;;
  all)    build; tables; width; config; search ;;
  *) echo "unknown group: $1 (tables|width|config|search|all)"; exit 2 ;;
esac
