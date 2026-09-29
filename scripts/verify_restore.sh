#!/usr/bin/env bash
# Read-only validation of a separately restored, isolated PostgreSQL database.
set -euo pipefail

: "${KUBEPILOT_RESTORE_DSN:?set KUBEPILOT_RESTORE_DSN to the isolated restore database}"

database=$(psql "$KUBEPILOT_RESTORE_DSN" -XAt -v ON_ERROR_STOP=1 -c 'SELECT current_database()')
case "$database" in
  kubepilot_restore_*) ;;
  *) echo 'Refusing to inspect a database not named kubepilot_restore_*' >&2; exit 1 ;;
esac

for table in users roles clusters agent_actions agent_action_audits audit_logs; do
  count=$(psql "$KUBEPILOT_RESTORE_DSN" -XAt -v ON_ERROR_STOP=1 -c "SELECT count(*) FROM $table")
  case "$count" in ''|*[!0-9]*) echo "Invalid count for $table" >&2; exit 1 ;; esac
  printf '%s=%s\n' "$table" "$count"
  if { [ "$table" = users ] || [ "$table" = roles ]; } && [ "$count" -eq 0 ]; then
    echo "Restore is missing required $table rows" >&2
    exit 1
  fi
done

echo 'Schema and basic row checks passed. Compare counts with the backup point and test login/decryption in a disposable KubePilot instance.'
