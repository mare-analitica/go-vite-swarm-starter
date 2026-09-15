#!/bin/sh
# n8n entrypoint: imports and publishes the example workflow on the first boot
# only (a marker in the data volume), so editor changes are never overwritten.
set -eu

marker=/home/node/.n8n/.starter-workflows-imported
if [ ! -f "$marker" ]; then
    n8n import:workflow --input=/workflows/note-created.json
    n8n publish:workflow --id=starterNoteCreated
    touch "$marker"
fi
exec n8n start
