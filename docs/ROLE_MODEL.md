# PostgreSQL role model

P03-T03. The database ships its security policy atomically with the schema
it protects: migration `00003_role_model.sql` creates the core roles, and
`scripts/dev-role-cycle.sh` manages the login credentials that belong to
operations rather than to the schema. Production provisions the same roles
through its own secret management; the passwords are never hardcoded in
SQL or fixtures.

## Roles

| Role | Created by | Login | Authority |
| --- | --- | --- | --- |
| `arena_owner` | migration 00003 | NOLOGIN | Owns every object; DDL authority. The bootstrap superuser relinquishes ownership after provisioning. |
| `arena_migrator` | migration 00003 | NOLOGIN | Runs the forward-only migration workflow; member of `arena_owner` (`SET role` is transient, exits with the migration session). |
| `arena_app` | migration 00003 (login credential by `dev-role-cycle.sh`) | LOGIN | The runtime of the application: data DML inside the `app` schema only. |
| `arena_backup` | `dev-role-cycle.sh` (operations, not schema) | LOGIN | Read-only dump authority over the `app` schema. |
| `arena_observability` | `dev-role-cycle.sh` (operations, not schema) | LOGIN | Read-only catalog and metrics authority (`pg_stat_*` views). |

## Invariants (enforced by the negative-test matrix)

1. **Runtime is least-privilege.** `arena_app` is not a superuser, cannot
   create roles or databases, and cannot create or alter schema objects:
   no `CREATE/ALTER/DROP` on `app` or on the migration history.
2. **Runtime cannot rewrite history.** `arena_app` gets only `SELECT` on
   `app.schema_metadata` (the goose version table): it may read which
   migrations are applied, but cannot `UPDATE`, `DELETE` or `TRUNCATE` the
   history, and cannot `CREATE TABLE` in `app`.
3. **Migrator is transient.** `arena_migrator` holds no standing
   privileges of its own beyond membership; migration sessions `SET role
   arena_owner` and exit. It never owns an object itself.
4. **Operational roles read, never write.** `arena_backup` and
   `arena_observability` cannot insert, update or delete anything; the
   observability role can always read catalogs, which are world-readable
   by design.

## Runtime grants (the exact current surface)

- `USAGE` on schema `app`
- `SELECT` on `app.schema_metadata`
- (further data tables and sequences join here in later phases with their
  own migrations)

## Credential handling

- Migration SQL contains no passwords.
- `scripts/dev-role-cycle.sh provision|rotate` generates passwords with
  `openssl rand`, applies them with `ALTER ROLE ... PASSWORD` and echoes
  them once for the local operator.
- Production injects credentials from the operator's secret store; the
  application reads them from `ARENA_DATABASE_URL` (redacted in logs and
  `String()` output by the typed config).

## Negative-test matrix

`scripts/dev-roles-negative-tests.sh` runs one SQL probe per forbidden
action and requires every probe to fail:

| Probe | Expected failure |
| --- | --- |
| `arena_app`: `CREATE TABLE app.app_forbidden()` | permission denied for schema app |
| `arena_app`: `CREATE SCHEMA app2` | permission denied for database |
| `arena_app`: `UPDATE app.schema_metadata` | permission denied for table |
| `arena_app`: `DELETE FROM app.schema_metadata` | permission denied for table |
| `arena_app`: `DROP TABLE app.schema_metadata` | must be owner of table |
| `arena_app`: `CREATE ROLE sneaky LOGIN` | permission denied |
| `arena_backup`: `INSERT INTO app.schema_metadata` | permission denied |
| `arena_observability`: `INSERT INTO app.schema_metadata` | permission denied |
