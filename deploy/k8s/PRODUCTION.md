# Production deployment and recovery gate

`production.yaml` is a template, not a ready-to-apply installation. Replace the
example image digest, database/Redis hosts, public hostname, ingress class and
namespace selector. Do not commit credentials or real kubeconfigs.

Prerequisites:

1. Build and scan the root `Dockerfile`; publish an immutable digest. The
   production manifest uses two non-root replicas and no hostPath mounts.
2. Create `kubepilot-prod-credentials` outside Git with `JWT_SECRET`,
   `ENCRYPT_KEY`, `DATABASE_PASSWORD`, `REDIS_PASSWORD` and
   `BOOTSTRAP_ADMIN_PASSWORD`. Use unique random values (at least 16 characters
   for JWT/encryption keys and 12 for bootstrap admin). Preserve `ENCRYPT_KEY`
   alongside database backups; losing it makes encrypted kubeconfigs and
   provider credentials unrecoverable.
3. Create `kubepilot-db-ca` with `ca.crt` and `kubepilot-tls` for the public
   hostname. PostgreSQL must offer TLS hostname verification. Redis must offer
   authenticated TLS. Restrict both to the application network.
4. Use an external PostgreSQL service with continuous WAL archiving/PITR and
   backups stored outside its failure domain. Velero workload backups do **not**
   protect the KubePilot database. Do not use the local hostPath examples for
   production. Redis should likewise be highly available.
5. Classify every managed cluster as production, staging or development in
   the cluster editor. New and migrated clusters default to production. The
   optional two-person gate is **off by default**; enable it in AI Settings
   only after assigning independent approvers with target-cluster write access.

When enabled, the gate blocks direct production workload/ops writes, terminal
access, tenant namespace changes and scheduler task execution, including jobs
queued before the switch was enabled. Agent-staged production Deployment
create/scale/update is the supported approval path; other resources require a
separate approved runbook. Existing in-flight Kubernetes operations cannot be
retroactively cancelled. When disabled, direct writes still require RBAC and
cluster grants, while Agent changes still require requester confirmation.

Every pull request and main-branch push runs Go tests, frontend build and a
runtime-image vulnerability scan. A `v*` tag publishes the scanned image to
GHCR with both commit and version tags; the workflow prints its immutable
digest. Pin `production.yaml` to that digest, not a mutable tag. Protect main
and release tags in GitHub settings; the workflow alone does not enforce those
repository policies. Do not roll out when the CI scan or restore drill fails.

Before first use and after every upgrade, perform a restore drill against an
isolated PostgreSQL instance: restore a base backup and WAL to a chosen point,
compare counts and representative rows from `users`, `roles`, `clusters`,
`agent_actions` and `agent_action_audits`, then start a disposable KubePilot
instance with the backed-up `ENCRYPT_KEY` and verify login, kubeconfig decrypt,
conversation history and audit export. Record backup timestamp, chosen recovery
point, elapsed restore time and any missing records. Agree an RPO/RTO with the
customer (for example 15 minutes / 2 hours) and fail the release gate when the
drill misses it. Restore into a separate database; never run a drill over the
live database.

After restoring into a separately named `kubepilot_restore_*` database, set
`KUBEPILOT_RESTORE_DSN` to that isolated database and run
`bash scripts/verify_restore.sh`. It checks required tables and baseline row
presence without modifying the database. This is a safety check, **not** a
substitute for comparing backup-point counts, WAL/PITR recovery, disposable-app
login and kubeconfig decryption. Record those results in the release evidence.

Audit requests return `X-Request-ID`; mutation metadata is also written as
`audit_event` JSON to stdout. Ship production stdout to external immutable
storage and poll `/api/v1/system/audit-logs/export?after_id=<last_id>` with
an account permitted to view audit logs. Persist `X-Next-Audit-ID` only after
the NDJSON page has been durably archived. Neither sink includes raw prompts,
YAML, tool output or request bodies.

The PostgreSQL advisory lock elects one instance for backup/inspection/event
watchers and serializes startup migrations. This prevents simultaneous
singleton workers, but does not promise exactly-once external side effects
after a crash. Keep external jobs idempotent and monitor duplicate delivery.
Backup and inspection schedule edits on standby replicas reach the elected
leader through database reconciliation within 15 seconds. Use at least two
nodes and an external database/Redis to survive a node loss; two Pods on one
node do not provide infrastructure high availability.
Schema changes are still GORM AutoMigrate, not versioned rollback scripts;
test upgrades and restore on a copy before every production rollout.
