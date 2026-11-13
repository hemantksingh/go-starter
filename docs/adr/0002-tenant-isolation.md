# ADR 0002: Multi-tenancy with a database per tenant

**Status:** accepted, 2026-11-13

## Context

The API serves multiple tenants. Each request is resolved to a tenant once, by middleware, and
the tenant's configuration and (from Phase 3) its database connection pool are placed on the
request context for everything downstream.

The isolation model decides how tenant data is kept apart:

- **Shared database, `tenant_id` column.** One schema, one pool, every query filtered by tenant.
  Optionally enforced by Postgres row-level security.
- **Schema per tenant.** One database, one pool, `search_path` switched per request.
- **Database per tenant.** Separate databases, one pool each, selected per request.

## Decision

**Database per tenant.** The tenant registry owns one connection pool per tenant, opened eagerly
at startup and closed on shutdown. The tenant middleware looks the tenant up by the
`X-Tenant-ID` header and puts the `tenant.Tenant` value, pool included, on the context. No code
below the middleware ever chooses a database; it uses the one it was handed.

## Rationale

- **Isolation by construction.** A query cannot reach another tenant's rows because the
  connection it runs on cannot see them. There is no `WHERE tenant_id = ?` to forget, and no
  policy to misconfigure.
- **Per-tenant operations are natural.** Backup, restore, migrate, or delete one tenant without
  touching the others.
- **Tenants are few and known.** They are loaded from a file. The pool-per-tenant cost is
  bounded and visible.

## Consequences

- **Connections scale with tenants.** Each pool holds up to `DB_MAX_CONNS`. Ten tenants at ten
  connections is a hundred, which is Postgres's default `max_connections`. The registry is the
  place to enforce a budget if the tenant count grows; this model is not the right one for
  thousands of tenants.
- **Migrations run once per database.** Startup (or a migrate command) loops over every
  tenant. The loop can half-fail and must report which tenants are behind.
- **No cross-tenant queries.** Any admin or reporting view fans out across pools in application
  code. That is the isolation being bought.
- **Tenant identity comes from the request, not the token.** The tenant wrapper runs before any
  authentication, so unauthenticated read routes can still be tenant-scoped. When JWT arrives in
  Phase 4, a tenant claim in the token must match the header.
- **Phase 2 ships the shape without the database.** The `DB` field on `tenant.Tenant` is nil
  until Phase 3; the registry, lookup, and context plumbing are built now so the pool slots in
  without changing any handler.

## Alternatives rejected

- **Shared database with `tenant_id`.** Cheapest to run and scales to many tenants, but
  isolation depends on every query being written correctly (or on row-level security being
  configured correctly). Reasonable for a SaaS with thousands of small tenants; not what this
  service is for.
- **Schema per tenant.** Middle ground, but `search_path` switching per request on a shared pool
  is a classic source of subtle bugs (a connection returned to the pool with the wrong path).
