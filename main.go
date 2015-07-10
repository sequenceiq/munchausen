package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"os"
)

var (
	flDebug = cli.BoolFlag{
		Name:   "debug",
		Usage:  "debug mode",
		EnvVar: "DEBUG",
	}
	flConsulServers = cli.StringFlag{
		Name:  "consulServers",
		Value: "",
		Usage: "IP addresses of consul servers as a comma separated list",
	}
	flConsulJoin = cli.StringFlag{
		Name:  "join",
		Value: "",
		Usage: "The consul cluster to join (consul://<consul_addr>:<port>)",
	}
	flWait = cli.IntFlag{
		Name:  "wait",
		Usage: "Waits approximately this many seconds for the docker daemons to start. Useful when Munchausen is started before all the docker daemons are started",
	}
	flConsulLogLocation = cli.StringFlag{
		Name: "consulLogLocation",
		Value: "",
		Usage: "Consul log location on local filesystem",
	}
	flFallbackDNSRecursors = cli.StringFlag{
		Name: "fallbackDNSRecursors",
		Value: "",
		Usage: "Additional DNS recursors to Consul config as a comma separated list",
	}
)

func main() {
	app := cli.NewApp()
	app.Name = "munchausen"
	app.Usage = "Bootstraps a Swarm cluster with Consul"
	app.Version = "0.5.4"
	app.Author = "SequenceIQ"

	app.Flags = []cli.Flag{
		flDebug,
	}

	app.Before = func(c *cli.Context) error {
		log.SetOutput(os.Stderr)
		if c.Bool(flDebug.Name) {
			log.SetLevel(log.DebugLevel)
		}
		return nil
	}

	app.Commands = []cli.Command{
		{
			Name:      "bootstrap",
			ShortName: "b",
			Usage:     "Bootstraps the cluster. A comma separated list of Docker daemon addresses must be passed as the first argument.",
			Flags:     []cli.Flag{flConsulServers, flWait, flConsulLogLocation, flFallbackDNSRecursors},
			Action:    bootstrap,
		},
		{
			Name:      "add",
			ShortName: "a",
			Usage:     "Adds new nodes to a Consul based Swarm cluster. A comma separated list of the new Docker daemons must be passed as the first argument.",
			Flags:     []cli.Flag{flConsulJoin, flWait, flConsulLogLocation, flFallbackDNSRecursors},
			Action:    add,
		},
	}

	app.Run(os.Args)
}
