-- Agent protocol metadata is written by the authenticated heartbeat.  A zero
-- API version means a legacy Agent that predates capability negotiation.
ALTER TABLE xcloud_nodes ADD COLUMN agent_version VARCHAR(64) NULL;
ALTER TABLE xcloud_nodes ADD COLUMN agent_api_version INT NOT NULL DEFAULT 0;
ALTER TABLE xcloud_nodes ADD COLUMN agent_capabilities JSON NULL;
