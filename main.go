package main

import (
	cloud "xcloud/src"
)

var Version = "dev"

func main() {
	cloud.Version = Version
	cloud.Run()
}
