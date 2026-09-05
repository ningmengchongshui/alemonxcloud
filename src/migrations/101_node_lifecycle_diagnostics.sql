-- Keep the latest Agent failure available to operators without exposing it to
-- customer-facing instance APIs.
ALTER TABLE xcloud_nodes ADD COLUMN last_agent_error VARCHAR(1024) NULL;
