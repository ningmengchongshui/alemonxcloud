#!/bin/bash
set -e

find . -name '.DS_Store' -type f -delete
yarn --cwd frontend build
server_binary=$(mktemp)
agent_binary=$(mktemp)
trap 'rm -f "$server_binary" "$agent_binary"' EXIT

go build -o "$server_binary" ./src
(
  cd agent
  go build -o "$agent_binary" .
)
