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
   the cluster editor. New and migrated clusters default to production, so AI
   writes require an independent approver with target-cluster write access.

The two-person gate currently applies to Agent-staged writes, not every manual
Kubernetes write endpoint. Production auto-execution is limited to Deployment
create/scale/update without hostPath mounts or environment-value changes.
Plan a separate approved runbook for other resources and verify that RBAC
blocks unauthorized direct writes.

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
