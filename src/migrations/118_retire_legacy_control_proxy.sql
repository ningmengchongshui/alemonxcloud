-- xcloud-control is now a self-hosted Agent. The old configurable local URL
-- proxy must not remain reachable after this release.
UPDATE xcloud_control_routes SET target_url='',status='disabled',updated_at=NOW() WHERE status<>'disabled' OR target_url<>'';
UPDATE xcloud_control_devices SET status='rebind_required',rebind_required=TRUE,credential_version=credential_version+1,updated_at=NOW() WHERE status='enabled' AND key_version<3;
INSERT INTO xcloud_control_events (device_id,event_type,detail,created_at)
SELECT id,'legacy_proxy_retired','旧本地目标入口已停用，需要使用新版 xcloud-control 重新接入',NOW() FROM xcloud_control_devices WHERE status='rebind_required';
