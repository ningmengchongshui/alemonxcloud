-- Existing images predate the access-mode flag. Keep their previous safe
-- behavior by exposing terminal access only until an administrator explicitly
-- enables the Web service entry in Software Settings.
ALTER TABLE xcloud_images ALTER COLUMN terminal_only SET DEFAULT TRUE;
UPDATE xcloud_images SET terminal_only=TRUE WHERE terminal_only=FALSE;
