module xcloud/control

go 1.24.4

require (
	github.com/creack/pty v1.1.24
	github.com/gorilla/websocket v1.5.3
	xcloud/agent-core v0.0.0
)

replace xcloud/agent-core => ../agent-core
