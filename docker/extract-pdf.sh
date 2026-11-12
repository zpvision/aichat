#!/bin/sh
set -eu
input_file=$(mktemp /tmp/knowledge.XXXXXX)
trap 'rm -f "$input_file"' EXIT
cat > "$input_file"
page_count=$(pdfinfo "$input_file" 2>/dev/null | awk '/^Pages:/ {print $2}')
case "$page_count" in ''|*[!0-9]*) exit 2 ;; esac
if [ "$page_count" -gt 500 ]; then exit 42; fi
pdftotext -enc UTF-8 -layout "$input_file" -
