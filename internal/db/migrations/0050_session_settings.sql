-- phase: expand
-- SPDX-License-Identifier: Apache-2.0
-- Spec 063. The pool asks every session to plan with its values and not to compile plans
-- (spec 061), but it asks at connect, and PgBouncer in transaction mode drops what a client
-- sends at connect. The database itself holds both settings, so every new session has them
-- however it arrives. A name cannot be a parameter, so the database is named with
-- format('%I'). Only the owner of a database may set its defaults: on a managed database
-- owned by someone else this notes it and goes on, and a direct connection still gets
-- both from the pool.
DO $$
BEGIN
  EXECUTE format('ALTER DATABASE %I SET plan_cache_mode = force_custom_plan', current_database());
  EXECUTE format('ALTER DATABASE %I SET jit = off', current_database());
EXCEPTION WHEN insufficient_privilege THEN
  RAISE NOTICE 'diffluence: not the owner of the database, so plan_cache_mode and jit are only sent at connect';
END
$$;
