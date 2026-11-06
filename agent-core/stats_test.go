package agentcore

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestParseContainerStats(t *testing.T) {
	stats, err := ParseContainerStats([]byte(`{"CPUPerc":"150.25%","MemUsage":"32MiB / 1GiB","MemPerc":"3.12%","NetIO":"10kB / 20kB","BlockIO":"0B / 1MB","PIDs":"4","Name":"private-host-name"}`))
	if err != nil {
		t.Fatal(err)
	}
	if stats.CPUPercent != "150.25%" || stats.MemoryUsage != "32MiB / 1GiB" || stats.NetworkIO != "10kB / 20kB" || stats.BlockIO != "0B / 1MB" || stats.PIDs != "4" {
		t.Fatalf("incorrect counters: %+v", stats)
	}
	if _, err := time.Parse(time.RFC3339, stats.SampledAt); err != nil {
		t.Fatal(err)
	}
	for _, raw := range []string{"", "{}", "invalid", `{"CPUPerc":""}`} {
		if _, err := ParseContainerStats([]byte(raw)); err == nil {
			t.Fatalf("accepted incomplete sample %q", raw)
		}
	}
}
func TestStatsRejectUnmanagedName(t *testing.T) {
	if _, err := ReadContainerStats(context.Background(), "--all"); err == nil {
		t.Fatal("accepted unsafe name")
	}
}

func TestReadContainerStatsCommand(t *testing.T) {
	dir := t.TempDir()
	executable := filepath.Join(dir, "docker")
	script := `#!/bin/sh
if [ "$1" != stats ] || [ "$2" != --no-stream ] || [ "$3" != --format ] || [ "$4" != '{{json .}}' ] || [ "$5" != xcloud-0123456789abcdef ]; then exit 3; fi
printf '%s' '{"CPUPerc":"42.00%","MemUsage":"128MiB / 1GiB","MemPerc":"12.50%","NetIO":"1MB / 2MB","BlockIO":"0B / 3MB","PIDs":"2"}'
`
	if err := os.WriteFile(executable, []byte(script), 0700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
	got, err := ReadContainerStats(context.Background(), "xcloud-0123456789abcdef")
	if err != nil || got.CPUPercent != "42.00%" {
		t.Fatalf("sample: %+v, %v", got, err)
	}
	if err := os.WriteFile(executable, []byte("#!/bin/sh\necho private-host-secret >&2\nexit 1\n"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadContainerStats(context.Background(), "xcloud-0123456789abcdef"); err == nil || strings.Contains(err.Error(), "private-host-secret") {
		t.Fatalf("unsafe failure: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadContainerStats(ctx, "xcloud-0123456789abcdef"); err == nil {
		t.Fatal("ignored cancellation")
	}
}
