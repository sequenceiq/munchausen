package swarmboot

import (
	"github.com/codegangsta/cli"
)

var (
	FlDebug = cli.BoolFlag{
		Name:   "debug",
		Usage:  "debug mode",
		EnvVar: "DEBUG",
	}
	FlDockerDaemonHost = cli.StringFlag{
		Name:  "dockerHost",
		Value: "",
		Usage: "Docker daemon URL t",
	}
	FlDockerDaemonPort = cli.IntFlag{
		Name:  "dockerPort",
		Value: 2376,
		Usage: "Docker daemon URL t",
	}
	FlConsulServers = cli.StringFlag{
		Name:  "consulServers",
		Value: "",
		Usage: "IP addresses of consul servers as a comma separated list",
	}
	FlConsulJoin = cli.StringFlag{
		Name:  "join",
		Value: "",
		Usage: "The consul cluster to join (consul://<consul_addr>:<port>)",
	}
	FlWait = cli.IntFlag{
		Name:  "wait",
		Usage: "Waits approximately this many seconds for the docker daemons to start. Useful when Munchausen is started before all the docker daemons are started",
	}
	FlFallbackDNSRecursors = cli.StringFlag{
		Name:  "fallbackDNSRecursors",
		Value: "",
		Usage: "Additional DNS recursors to Consul config as a comma separated list",
	}
)
