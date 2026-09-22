# Relatório da auditoria de migrations

Gerado por `tools/migrationaudit` no commit 62e1eb3, em 2026-09-22T15:22:38Z, em 10.744s.

O exercício roda contra um PostgreSQL descartável: um banco construído do zero, um degrau que aplica cada migration com um leitor ativo, um snapshot copiado e rolado para o head em **cada** versão, uma migration que falha pela metade e a recuperação do banco em que ela falhou.

## 1. Banco vazio

A referência: a história inteira aplicada por `dbmigrate.Up` numa base vazia. É contra este banco que todo snapshot é comparado.

| Versões | Relações | Linhas |
| --- | --- | --- |
| 46 | 46 | 41 |

## 2. Degrau: uma migration por vez, com um leitor ativo

Enquanto cada migration é aplicada, outra sessão mantém `ACCESS SHARE` — o lock de um `SELECT` — em todas as tabelas do schema. "Esperou" significa que a migration não terminaria enquanto a aplicação estivesse lendo: é a estimativa de lock que o `docs/DEPLOYMENT.md` §6 exige para uma janela.

| Versão | Arquivo | Tempo | Esperou por leitor | Bloqueio observado | Tabelas semeadas |
| --- | --- | --- | --- | --- | --- |
| 1 | `00001_app_schema.sql` | 7.957ms | não | — | 0 |
| 2 | `00002_app_schema_metadata_comments.sql` | 5.843ms | não | — | 0 |
| 3 | `00003_role_model.sql` | 17.112ms | **sim** | `app.schema_metadata AccessExclusiveLock` | 0 |
| 4 | `00004_identity_schema.sql` | 34.904ms | não | — | 5 |
| 5 | `00005_profiles_schema.sql` | 11.587ms | não | — | 2 |
| 6 | `00006_communication_preferences.sql` | 8.886ms | não | — | 2 |
| 7 | `00007_profile_timezone.sql` | 18.228ms | **sim** | `app.profiles AccessExclusiveLock` | 0 |
| 8 | `00008_wallet_ledger_schema.sql` | 12.234ms | não | — | 3 |
| 9 | `00009_wallet_free_cycle_anchor.sql` | 17.226ms | **sim** | `app.wallet_accounts AccessExclusiveLock` | 0 |
| 10 | `00010_wallet_admin_audit_columns.sql` | 19.422ms | **sim** | `app.wallet_operations AccessExclusiveLock` | 0 |
| 11 | `00011_arena_pass_schema.sql` | 10.157ms | não | — | 2 |
| 12 | `00012_arena_schema.sql` | 13.573ms | não | — | 3 |
| 13 | `00013_arena_public_feed_index.sql` | 6.548ms | não | — | 0 |
| 14 | `00014_position_schema.sql` | 12.231ms | não | — | 2 |
| 15 | `00015_argument_schema.sql` | 15.042ms | não | — | 2 |
| 16 | `00016_argument_idempotency_key.sql` | 18.518ms | **sim** | `app.arguments AccessExclusiveLock` | 0 |
| 17 | `00017_argument_withdrawn_at.sql` | 17.139ms | **sim** | `app.arguments AccessExclusiveLock` | 0 |
| 18 | `00018_persuasion_attribution_schema.sql` | 23.303ms | **sim** | `app.position_changes AccessExclusiveLock` | 1 |
| 19 | `00019_persuasion_attribution_moderation.sql` | 19.577ms | **sim** | `app.persuasion_attributions AccessExclusiveLock` | 0 |
| 20 | `00020_billing_stripe_schema.sql` | 24ms | não | — | 6 |
| 21 | `00021_billing_refunds.sql` | 24.069ms | **sim** | `app.wallet_operations AccessExclusiveLock` | 1 |
| 22 | `00022_moderation_schema.sql` | 24.611ms | não | — | 5 |
| 23 | `00023_admin_security_role.sql` | 18.94ms | **sim** | `app.admin_roles AccessExclusiveLock` | 0 |
| 24 | `00024_moderation_claim_lease.sql` | 20.497ms | **sim** | `app.moderation_cases AccessExclusiveLock` | 0 |
| 25 | `00025_audit_events.sql` | 11.546ms | não | — | 1 |
| 26 | `00026_personal_exports.sql` | 13.924ms | não | — | 1 |
| 27 | `00027_account_deletion.sql` | 15.74ms | não | — | 1 |
| 28 | `00028_data_retention.sql` | 16.635ms | não | — | 2 |
| 29 | `00029_jobs.sql` | 13.713ms | não | — | 1 |
| 30 | `00030_admin_mfa.sql` | 18.435ms | **sim** | `app.sessions AccessExclusiveLock` | 2 |
| 31 | `00031_rebuildable_stat_projections.sql` | 11.392ms | não | — | 2 |
| 32 | `00032_full_text_search.sql` | 7.522ms | não | — | 0 |

## 3. Snapshot e upgrade de cada versão

Depois de cada versão o banco foi copiado e a cópia rolou para o head com as migrations restantes. O resultado tem de ser **o mesmo banco** que o banco vazio produziu, sem perder linha, e sem perder coluna que a versão do snapshot já tinha.

| Snapshot da versão | Migrations restantes | Tempo | Mesmo schema? | Colunas perdidas | Linhas perdidas | História completa? |
| --- | --- | --- | --- | --- | --- | --- |
| 2 | 30 | 0s | sim | — | — | sim |
| 3 | 29 | 0s | sim | — | — | sim |
| 4 | 28 | 0s | sim | — | — | sim |
| 5 | 27 | 0s | sim | — | — | sim |
| 6 | 26 | 0s | sim | — | — | sim |
| 7 | 25 | 0s | sim | — | — | sim |
| 8 | 24 | 0s | sim | — | — | sim |
| 9 | 23 | 0s | sim | — | — | sim |
| 10 | 22 | 0s | sim | — | — | sim |
| 11 | 21 | 0s | sim | — | — | sim |
| 12 | 20 | 0s | sim | — | — | sim |
| 13 | 19 | 0s | sim | — | — | sim |
| 14 | 18 | 0s | sim | — | — | sim |
| 15 | 17 | 0s | sim | — | — | sim |
| 16 | 16 | 0s | sim | — | — | sim |
| 17 | 15 | 0s | sim | — | — | sim |
| 18 | 14 | 0s | sim | — | — | sim |
| 19 | 13 | 0s | sim | — | — | sim |
| 20 | 12 | 0s | sim | — | — | sim |
| 21 | 11 | 0s | sim | — | — | sim |
| 22 | 10 | 0s | sim | — | — | sim |
| 23 | 9 | 0s | sim | — | — | sim |
| 24 | 8 | 0s | sim | — | — | sim |
| 25 | 7 | 0s | sim | — | — | sim |
| 26 | 6 | 0s | sim | — | — | sim |
| 27 | 5 | 0s | sim | — | — | sim |
| 28 | 4 | 0s | sim | — | — | sim |
| 29 | 3 | 0s | sim | — | — | sim |
| 30 | 2 | 0s | sim | — | — | sim |
| 31 | 1 | 0s | sim | — | — | sim |
| 32 | 0 | 0s | sim | — | — | sim |

## 4. Falha simulada e recuperação

Na versão 17, uma migration injetada criou uma tabela, inseriu uma linha e então dividiu por zero. O runner tem de desfazer **tudo** — a tabela, a linha e o registro de versão — e o banco tem de continuar rolável para o head, porque `migrate down` não existe no runner: a volta operacional é o processo e o schema segue em frente.

A migration injetada falhou num banco que ainda tinha migrations reais pendentes: 0 delas foram aplicadas antes dela.

| Erro | Versão da falha registrada? | Sobras | Relações perdidas | Linhas perdidas | Migrations restantes na recuperação | Recuperou? |
| --- | --- | --- | --- | --- | --- | --- |
| `partial migration error (type:sql,version:90001): ERROR: division by zero (SQLSTATE 22012)` | **não** | — | — | — | 0 | sim |

## 5. Dataset

O audit escreve uma linha por tabela que aceita `INSERT ... DEFAULT VALUES`, a cada versão, e verifica em cada upgrade que nenhuma linha desapareceu. Um `INSERT` que não conhece o domínio não é um dataset de produção: é a prova de que a linha **atravessa** a história.

| Tabelas semeadas | Linhas | Piso | Não semeadas |
| --- | --- | --- | --- |
| 2 | 41 | 2 | 45 |

As tabelas que o seed ingênuo não consegue preencher, e por quê:

| Tabela | Motivo |
| --- | --- |
| `account_deletion_requests` | `ERROR: null value in column "account_id" of relation "account_deletion_requests" violates not-null constraint (SQLSTATE 23502)` |
| `account_mfa` | `ERROR: null value in column "account_id" of relation "account_mfa" violates not-null constraint (SQLSTATE 23502)` |
| `accounts` | `ERROR: null value in column "email" of relation "accounts" violates not-null constraint (SQLSTATE 23502)` |
| `admin_roles` | `ERROR: null value in column "account_id" of relation "admin_roles" violates not-null constraint (SQLSTATE 23502)` |
| `arena_pass_consumptions` | `ERROR: null value in column "lot_id" of relation "arena_pass_consumptions" violates not-null constraint (SQLSTATE 23502)` |
| `arena_pass_lots` | `ERROR: null value in column "account_id" of relation "arena_pass_lots" violates not-null constraint (SQLSTATE 23502)` |
| `arena_public_stat_projections` | `ERROR: null value in column "arena_id" of relation "arena_public_stat_projections" violates not-null constraint (SQLSTATE 23502)` |
| `arena_relations` | `ERROR: null value in column "arena_id" of relation "arena_relations" violates not-null constraint (SQLSTATE 23502)` |
| `arenas` | `ERROR: null value in column "creator_id" of relation "arenas" violates not-null constraint (SQLSTATE 23502)` |
| `argument_sources` | `ERROR: null value in column "argument_id" of relation "argument_sources" violates not-null constraint (SQLSTATE 23502)` |
| `arguments` | `ERROR: null value in column "arena_id" of relation "arguments" violates not-null constraint (SQLSTATE 23502)` |
| `audit_events` | `ERROR: null value in column "actor_id" of relation "audit_events" violates not-null constraint (SQLSTATE 23502)` |
| `billing_reconciliation_findings` | `ERROR: null value in column "run_id" of relation "billing_reconciliation_findings" violates not-null constraint (SQLSTATE 23502)` |
| `billing_reconciliation_runs` | `ERROR: null value in column "livemode" of relation "billing_reconciliation_runs" violates not-null constraint (SQLSTATE 23502)` |
| `billing_refunds` | `ERROR: null value in column "account_id" of relation "billing_refunds" violates not-null constraint (SQLSTATE 23502)` |
| `categories` | `ERROR: null value in column "slug" of relation "categories" violates not-null constraint (SQLSTATE 23502)` |
| `checkout_intents` | `ERROR: null value in column "account_id" of relation "checkout_intents" violates not-null constraint (SQLSTATE 23502)` |
| `communication_preference_history` | `ERROR: null value in column "account_id" of relation "communication_preference_history" violates not-null constraint (SQLSTATE 23502)` |
| `communication_preferences` | `ERROR: null value in column "account_id" of relation "communication_preferences" violates not-null constraint (SQLSTATE 23502)` |
| `data_exports` | `ERROR: null value in column "account_id" of relation "data_exports" violates not-null constraint (SQLSTATE 23502)` |
| `debate_positions` | `ERROR: null value in column "arena_id" of relation "debate_positions" violates not-null constraint (SQLSTATE 23502)` |
| `email_verification_tokens` | `ERROR: null value in column "account_id" of relation "email_verification_tokens" violates not-null constraint (SQLSTATE 23502)` |
| `jobs` | `ERROR: null value in column "type" of relation "jobs" violates not-null constraint (SQLSTATE 23502)` |
| `mfa_backup_codes` | `ERROR: null value in column "account_id" of relation "mfa_backup_codes" violates not-null constraint (SQLSTATE 23502)` |
| `moderation_actions` | `ERROR: null value in column "case_id" of relation "moderation_actions" violates not-null constraint (SQLSTATE 23502)` |
| `moderation_appeals` | `ERROR: null value in column "action_id" of relation "moderation_appeals" violates not-null constraint (SQLSTATE 23502)` |
| `moderation_cases` | `ERROR: null value in column "target_type" of relation "moderation_cases" violates not-null constraint (SQLSTATE 23502)` |
| `moderation_reports` | `ERROR: null value in column "reporter_id" of relation "moderation_reports" violates not-null constraint (SQLSTATE 23502)` |
| `password_credentials` | `ERROR: null value in column "account_id" of relation "password_credentials" violates not-null constraint (SQLSTATE 23502)` |
| `password_reset_tokens` | `ERROR: null value in column "account_id" of relation "password_reset_tokens" violates not-null constraint (SQLSTATE 23502)` |
| `persuasion_attributions` | `ERROR: null value in column "position_change_id" of relation "persuasion_attributions" violates not-null constraint (SQLSTATE 23502)` |
| `position_changes` | `ERROR: null value in column "arena_id" of relation "position_changes" violates not-null constraint (SQLSTATE 23502)` |
| `profiles` | `ERROR: null value in column "account_id" of relation "profiles" violates not-null constraint (SQLSTATE 23502)` |
| `retention_holds` | `ERROR: null value in column "data_class" of relation "retention_holds" violates not-null constraint (SQLSTATE 23502)` |
| `retention_runs` | `ERROR: null value in column "data_class" of relation "retention_runs" violates not-null constraint (SQLSTATE 23502)` |
| `schema_metadata` | `ERROR: null value in column "version_id" of relation "schema_metadata" violates not-null constraint (SQLSTATE 23502)` |
| `sessions` | `ERROR: null value in column "account_id" of relation "sessions" violates not-null constraint (SQLSTATE 23502)` |
| `stripe_customers` | `ERROR: null value in column "account_id" of relation "stripe_customers" violates not-null constraint (SQLSTATE 23502)` |
| `stripe_events` | `ERROR: null value in column "stripe_event_id" of relation "stripe_events" violates not-null constraint (SQLSTATE 23502)` |
| `subscriptions` | `ERROR: null value in column "account_id" of relation "subscriptions" violates not-null constraint (SQLSTATE 23502)` |
| `transparency_stat_projections` | `ERROR: null value in column "period_start" of relation "transparency_stat_projections" violates not-null constraint (SQLSTATE 23502)` |
| `username_history` | `ERROR: null value in column "account_id" of relation "username_history" violates not-null constraint (SQLSTATE 23502)` |
| `wallet_accounts` | `ERROR: null value in column "account_id" of relation "wallet_accounts" violates not-null constraint (SQLSTATE 23502)` |
| `wallet_operations` | `ERROR: null value in column "account_id" of relation "wallet_operations" violates not-null constraint (SQLSTATE 23502)` |
| `wallet_transactions` | `ERROR: null value in column "operation_id" of relation "wallet_transactions" violates not-null constraint (SQLSTATE 23502)` |

## 6. Regras

| Regra | Gravidade | Objeto | Resultado | Detalhe |
| --- | --- | --- | --- | --- |
| `destructive-declared` | gate | statement | **passa** | a migration that removes something is recorded in the ledger with its reason:  |
| `destructive-ledger-fresh` | gate | ledger | **passa** | an entry of the ledger that no migration states is a decision that outlived its code:  |
| `blocking-declared` | gate | migration | **passa** | a migration that waits for an active reader needs a window and a line in the ledger:  |
| `blocking-ledger-fresh` | gate | ledger | **passa** | this ledger entry names a migration that no longer waits for a reader:  |
| `ladder-reaches-head` | gate | history | **passa** | the ladder stopped at version 32 after 32 step(s) |
| `snapshot-lands-on-virgin` | gate | snapshot | **passa** | a database upgraded from these snapshots is not the database a fresh install produces:  |
| `upgrade-keeps-rows` | gate | data | **passa** | an upgrade lost rows:  |
| `expansion-is-monotonic` | gate | structure | **passa** | a column or table the snapshot had is gone at head, and expand/contract means it stays until a later release removes it:  |
| `history-complete-after-upgrade` | gate | history | **passa** | the version table of an upgraded database does not list the history:  |
| `failure-rolls-back` | gate | failure | **passa** | the failed migration left state behind: recorded=false leftover=[] missing=[] lost-rows=[] |
| `failure-recovers` | gate | failure | **passa** | after the failure the history had 0 migration(s) to run and did not reach the database a fresh install produces |
| `owner-is-the-schema-owner` | gate | ownership | **passa** | every object of the schema belongs to arena_owner:  |
| `runtime-is-least-privilege` | gate | role | **passa** | arena_app: super=false createdb=false createrole=false bypassrls=false replication=false login=true |
| `schema-usage-without-create` | gate | role | **passa** | arena_app on schema app: usage=true create=false |
| `no-public-grants` | gate | grants | **passa** | PUBLIC holds privileges on the schema:  |
| `history-is-read-only-for-the-runtime` | gate | grants | **passa** | arena_app holds more than SELECT on the version table:  |
| `runtime-reads-every-table` | gate | grants | **passa** | arena_app cannot read:  |
| `runtime-writes-are-declared` | gate | grants | **passa** |  |
| `runtime-holds-no-structural-privilege` | gate | grants | **passa** | arena_app holds TRUNCATE, REFERENCES or TRIGGER on:  |
| `grant-ledgers-are-fresh` | gate | ledger | **passa** | the ledger classifies a table the schema no longer matches:  |
| `sequences-are-usable-and-immutable` | gate | grants | **passa** | arena_app must hold USAGE and SELECT exactly on the sequences of the tables it inserts into, and never UPDATE:  |
| `indexes-are-valid` | gate | indexes | **passa** | an invalid or not-ready index is a promise the planner cannot use:  |
| `indexes-are-not-duplicated` | gate | indexes | **passa** | two indexes on the same columns of the same table:  |
| `foreign-keys-are-local` | gate | foreign keys | **passa** | a foreign key leaves the schema:  |
| `cascades-are-declared` | gate | cascades | **passa** | deleting the parent deletes these rows and the ledger does not say why:  |
| `cascade-ledger-is-fresh` | gate | ledger | **passa** | the ledger names cascades the schema does not have:  |
| `foreign-keys-are-indexed` | advisory | foreign keys | **advertência** | 8 of 58 foreign key(s) have no index leading with their columns: admin_roles.admin_roles_granted_by_fkey (granted_by); arenas.arenas_category_fkey (category); moderation_appeals.moderation_appeals_reviewer_id_fkey (reviewer_id); moderation_cases.moderation_cases_claimed_by_fkey (claimed_by); persuasion_attributions.persuasion_attributions_moderated_by_fkey (moderated_by); retention_holds.retention_holds_account_id_fkey (account_id); retention_holds.retention_holds_placed_by_fkey (placed_by); wallet_operations.wallet_operations_actor_account_id_fkey (actor_account_id) (partially covered, leading column only: arguments.arguments_parent_same_arena_fk (parent_id,arena_id: parent_id leads an index); persuasion_attributions.persuasion_attributions_change_fk (position_change_id,attributor_id: position_change_id leads an index)) |
| `schema-matches-the-history` | gate | schema | **passa** | the database and the forward halves disagree about which tables exist (declared 45, catalog 45):  |
| `dataset-floor` | gate | dataset | **passa** | the audit could seed 2 table(s) and the floor is 2 |
| `dataset-travelled` | gate | dataset | **passa** | the audit wrote no row at all, so data survival would be vacuous |

## 6b. Substituições em vigor (o que a história derruba e recria no mesmo arquivo)

Um `DROP` seguido da recriação do mesmo objeto na mesma migration não é uma contração: o objeto continua lá, com outro corpo. A lista existe para que os 47 casos medidos sejam visíveis — e para que uma migration que derrube algo **sem** recriar apareça como falha do portão, e não como ruído nesta lista.

| Versão | Arquivo | Objeto |
| --- | --- | --- |
| 12 | `00012_arena_schema.sql` | `arenas_protect_published_update` |
| 12 | `00012_arena_schema.sql` | `arenas_protect_published_delete` |
| 14 | `00014_position_schema.sql` | `debate_positions_protect_initial_update` |
| 14 | `00014_position_schema.sql` | `position_changes_append_only_guard` |
| 15 | `00015_argument_schema.sql` | `arguments_protect_published_update` |
| 15 | `00015_argument_schema.sql` | `arguments_retained_delete` |
| 18 | `00018_persuasion_attribution_schema.sql` | `persuasion_attributions_protect_update` |
| 18 | `00018_persuasion_attribution_schema.sql` | `persuasion_attributions_retained_delete` |
| 19 | `00019_persuasion_attribution_moderation.sql` | `persuasion_attributions_decision_retained` |
| 20 | `00020_billing_stripe_schema.sql` | `stripe_customers_retained_delete` |
| 20 | `00020_billing_stripe_schema.sql` | `stripe_events_retained_delete` |
| 20 | `00020_billing_stripe_schema.sql` | `checkout_intents_retained_delete` |
| 20 | `00020_billing_stripe_schema.sql` | `subscriptions_retained_delete` |
| 20 | `00020_billing_stripe_schema.sql` | `billing_reconciliation_runs_retained_delete` |
| 20 | `00020_billing_stripe_schema.sql` | `billing_reconciliation_findings_retained_delete` |
| 20 | `00020_billing_stripe_schema.sql` | `stripe_customers_protect_mapping` |
| 20 | `00020_billing_stripe_schema.sql` | `stripe_events_protect_processing` |
| 20 | `00020_billing_stripe_schema.sql` | `checkout_intents_protect_lifecycle` |
| 20 | `00020_billing_stripe_schema.sql` | `subscriptions_protect_lifecycle` |
| 20 | `00020_billing_stripe_schema.sql` | `billing_reconciliation_findings_protect` |
| 21 | `00021_billing_refunds.sql` | `wallet_operations_type_check` |
| 21 | `00021_billing_refunds.sql` | `billing_refunds_retained_delete` |
| 21 | `00021_billing_refunds.sql` | `billing_refunds_protect` |
| 22 | `00022_moderation_schema.sql` | `admin_roles_retained_delete` |
| 22 | `00022_moderation_schema.sql` | `moderation_reports_retained_delete` |
| 22 | `00022_moderation_schema.sql` | `moderation_cases_retained_delete` |
| 22 | `00022_moderation_schema.sql` | `moderation_actions_retained_delete` |
| 22 | `00022_moderation_schema.sql` | `moderation_appeals_retained_delete` |
| 22 | `00022_moderation_schema.sql` | `admin_roles_protect_assignment` |
| 22 | `00022_moderation_schema.sql` | `moderation_reports_protect_evidence` |
| 22 | `00022_moderation_schema.sql` | `moderation_cases_protect_lifecycle` |
| 22 | `00022_moderation_schema.sql` | `moderation_actions_freeze` |
| 22 | `00022_moderation_schema.sql` | `moderation_appeals_protect` |
| 23 | `00023_admin_security_role.sql` | `admin_roles_role_check` |
| 25 | `00025_audit_events.sql` | `audit_events_freeze_update` |
| 25 | `00025_audit_events.sql` | `audit_events_freeze_delete` |
| 26 | `00026_personal_exports.sql` | `data_exports_protect_insert` |
| 26 | `00026_personal_exports.sql` | `data_exports_protect_update` |
| 26 | `00026_personal_exports.sql` | `data_exports_retained_delete` |
| 27 | `00027_account_deletion.sql` | `account_deletion_requests_guard_update` |
| 27 | `00027_account_deletion.sql` | `account_deletion_requests_retained_delete` |
| 28 | `00028_data_retention.sql` | `retention_holds_retained_delete` |
| 28 | `00028_data_retention.sql` | `retention_holds_protect_update` |
| 28 | `00028_data_retention.sql` | `retention_runs_freeze_update` |
| 28 | `00028_data_retention.sql` | `retention_runs_freeze_delete` |
| 29 | `00029_jobs.sql` | `jobs_freeze_provenance_update` |

## 7. Ledgers

As exceções declaradas: cada uma foi medida primeiro e justificada depois, e o audit falha quando a realidade deixa de casar com elas em qualquer direção.

**Contrações (o que uma migration do histórico remove)**

Nenhuma.

**Migrations que esperam por um leitor ativo (precisam de janela)**

| Chave | Motivo |
| --- | --- |
| `10` | ALTER TABLE app.wallet_operations adds the administrative audit columns (actor_account_id, reason) and the CHECK over them: the append-only ledger is locked for the duration |
| `16` | CREATE UNIQUE INDEX on app.arguments builds the idempotency key over the rows that already exist; CONCURRENTLY is not available to a migration that runs inside goose's transaction-per-migration |
| `17` | ALTER TABLE app.arguments adds withdrawn_at and the CHECK that reads it: every argument is locked |
| `18` | ALTER TABLE app.position_changes adds the attribution columns: the chain of every position is locked |
| `19` | ALTER TABLE app.persuasion_attributions adds the moderation decision columns and the protect trigger |
| `21` | ALTER TABLE app.wallet_operations replaces the CHECK that lists the operation types: a CHECK rewrite validates every existing row of the ledger |
| `23` | ALTER TABLE app.admin_roles replaces the capability CHECK to admit the security role |
| `24` | ALTER TABLE app.moderation_cases adds the claim lease columns and the partial index over them: the queue of open cases is locked |
| `3` | ALTER TABLE app.schema_metadata changes the owner of the version table: ownership takes ACCESS EXCLUSIVE, and the role model rewrites the grants of the whole schema in the same migration |
| `30` | ALTER TABLE app.sessions adds mfa_verified_at: every live session is locked for the duration of the statement |
| `7` | ALTER TABLE app.profiles adds the timezone column and the CHECK that reads it: the profile of every account is locked for the duration |
| `9` | ALTER TABLE app.wallet_accounts adds the free-cycle anchor: the balance projection of every account is locked |

**Cascades**

| Chave | Motivo |
| --- | --- |
| `account_mfa.account_mfa_account_id_fkey` | second factors are part of the credential, and the same reason applies |
| `arena_public_stat_projections.arena_public_stat_projections_arena_id_fkey` | the projection of an arena is derived data: it is rebuilt from the events it summarises (migration 31) |
| `communication_preference_history.communication_preference_history_account_id_fkey` | the preference history is the account's own trail, kept only as long as the account |
| `communication_preferences.communication_preferences_account_id_fkey` | a preference of a deleted account cannot be honoured and is not evidence |
| `email_verification_tokens.email_verification_tokens_account_id_fkey` | a verification token for a deleted account would verify an address nobody owns |
| `mfa_backup_codes.mfa_backup_codes_account_id_fkey` | one-use codes are part of the credential, and the same reason applies |
| `password_credentials.password_credentials_account_id_fkey` | a credential without its account authenticates nobody and is a secret that should not outlive it |
| `password_reset_tokens.password_reset_tokens_account_id_fkey` | a reset token for a deleted account is a live grant to a row that no longer exists |
| `profiles.profiles_account_id_fkey` | the profile is the account's public face: it has no identity of its own and the deletion journey (migration 27) removes both together |
| `sessions.sessions_account_id_fkey` | a session of a deleted account must not survive the account: it is the clearest reading of "the account is gone" |
| `username_history.username_history_account_id_fkey` | the history of a name release is personal data of the account that held it |

**Contrações: o que a história remove**

Nenhuma.

**Append-only para o runtime (INSERT + SELECT, nunca UPDATE)**

| Chave | Motivo |
| --- | --- |
| `arena_pass_consumptions` | a consumed pass is the consumption of something paid for: it is the row that proves the pass was used |
| `argument_sources` | the sources an argument cites: a citation is evidence, so it is added and withdrawn (there is a withdrawn_at column) and never rewritten |
| `audit_events` | the administrative audit trail: an event that can be edited is not evidence |
| `moderation_actions` | what a moderator did, when and why: the moderation history is read to justify decisions and cannot be revised |
| `moderation_reports` | what a user reported: the report is testimony and is closed by state, not by editing |
| `position_changes` | the chain of a position: each row is one transition, and the current position is the last row, never a rewritten one |
| `retention_runs` | each run of the retention sweep is a record of a deletion, which is exactly what must not be erasable |
| `wallet_operations` | the operations that move INK, with no UPDATE: a correction is a compensating operation (migration 21), never an edit |
| `wallet_transactions` | the ledger of INK: append-only by the plan, and the transaction is the immutable record a balance is derived from |

**Somente leitura para o runtime (SELECT apenas)**

| Chave | Motivo |
| --- | --- |
| `categories` | the arena categories are seeded by the schema: the product has no feature that creates one, and a category is a product decision |
| `schema_metadata` | the migration history itself, written by the owner through goose; the runtime reads the applied version and never writes it |

**Tabelas das quais o runtime pode apagar**

| Chave | Motivo |
| --- | --- |
| `accounts` | the deletion journey (migration 27) erases the account row itself |
| `arena_public_stat_projections` | a projection is rebuilt, so its rows are removed and rewritten by the rebuild |
| `arena_relations` | the relations between arenas are curated: a relation is removed when it stops being true |
| `arenas` | a draft arena is deleted (db/queries/arenas.sql), and the account erasure deletes the arenas of the account (db/queries/account_deletion.sql); the trigger arenas_published_not_deletable refuses the delete once the arena is published, closed or restricted, so the privilege exists and the schema narrows it |
| `communication_preference_history` | the trail is removed with the account |
| `communication_preferences` | the preference row is removed when the account is erased |
| `email_verification_tokens` | a consumed or expired token is deleted |
| `mfa_backup_codes` | a backup code is deleted when it is used |
| `password_credentials` | the credential is deleted with the account |
| `password_reset_tokens` | a consumed or expired token is deleted |
| `profiles` | the profile is deleted with its account |
| `sessions` | a session ends by deletion, on logout and on expiry |
| `transparency_stat_projections` | the same rebuild, for the public transparency page |
| `username_history` | the released name is forgotten with the account |

Piso do dataset: **2** tabela(s).

## 9. Advertências (registradas, não recusam)

Medições em que o schema é defensável e o achado é um trabalho seguinte. Estão aqui com o objeto exato e com o trabalho a que foram atribuídas — uma advertência sem dono é um achado que ninguém leva.

- foreign-keys-are-indexed: foreign keys: 8 of 58 foreign key(s) have no index leading with their columns: admin_roles.admin_roles_granted_by_fkey (granted_by); arenas.arenas_category_fkey (category); moderation_appeals.moderation_appeals_reviewer_id_fkey (reviewer_id); moderation_cases.moderation_cases_claimed_by_fkey (claimed_by); persuasion_attributions.persuasion_attributions_moderated_by_fkey (moderated_by); retention_holds.retention_holds_account_id_fkey (account_id); retention_holds.retention_holds_placed_by_fkey (placed_by); wallet_operations.wallet_operations_actor_account_id_fkey (actor_account_id) (partially covered, leading column only: arguments.arguments_parent_same_arena_fk (parent_id,arena_id: parent_id leads an index); persuasion_attributions.persuasion_attributions_change_fk (position_change_id,attributor_id: position_change_id leads an index))

| Regra | Trabalho seguinte |
| --- | --- |
| `foreign-keys-are-indexed` | uma migration que adicione os índices de cobertura para as chaves que este run lista, revisada por si: um índice muda o caminho de escrita da tabela e a janela da promoção, e não é algo que uma auditoria inclua no próprio commit |

