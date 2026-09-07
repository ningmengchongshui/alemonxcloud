package cloud

import "strings"

// tunnelInternalURL is the control-plane-only Gateway address. It carries
// structured commands, terminal WebSockets and self-hosted instance proxying.
// The two old variables are accepted only as a rollout bridge; new deployments
// must configure XCLOUD_TUNNEL_INTERNAL_URL.
func tunnelInternalURL() string {
	value := env("XCLOUD_TUNNEL_INTERNAL_URL", "")
	if value == "" {
		value = env("XCLOUD_TUNNEL_COMMAND_URL", env("XCLOUD_TUNNEL_PROXY_URL", ""))
	}
	return strings.TrimRight(value, "/")
}
