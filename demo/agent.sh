#!/bin/sh
# Stands in for the coding agent nit sends comments to: a moment after a
# "rename A to B" comment arrives, it makes that edit in the file.
req=$(cat)
file=$(printf '%s' "$req" | jq -r .file)
words=$(printf '%s' "$req" | jq -r .text |
	sed -n 's/^rename \([A-Za-z_][A-Za-z0-9_]*\) to \([A-Za-z_][A-Za-z0-9_]*\)$/\1 \2/p')
[ -n "$words" ] || exit 0
set -- $words
(sleep 2; perl -pi -e "s/\\b$1\\b/$2/g" "$NIT_ROOT/$file") </dev/null >/dev/null 2>&1 &
