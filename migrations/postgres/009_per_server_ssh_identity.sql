-- +goose Up
-- Per-server SSH identity.
--
-- Before this migration a single fleet-wide SSH_PASSWORD authenticated every
-- target, so one compromised credential exposed the whole inventory and there
-- was no way to rotate one host independently. These columns move the product
-- to a per-server credential reference and a pinned host key.
--
-- Only a reference is stored, never key material: the control plane should not
-- be able to log in to a customer's machines, so the private key stays in the
-- runner's own keystore.
--
-- Both columns are nullable and are left NULL for existing rows on purpose. A
-- server registered before this change has no verified host key, and the
-- runner refuses to execute against it until an operator records one. Failing
-- closed is the point: defaulting these would silently reintroduce unverified
-- connections.
ALTER TABLE servers ADD COLUMN IF NOT EXISTS ssh_credential_ref TEXT;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS ssh_host_key_fingerprint TEXT;

-- +goose Down
ALTER TABLE servers DROP COLUMN IF EXISTS ssh_host_key_fingerprint;
ALTER TABLE servers DROP COLUMN IF EXISTS ssh_credential_ref;
