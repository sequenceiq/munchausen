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
	flConsulServers = cli.IntFlag{
		Name:  "consulServers",
		Value: 3,
		Usage: "number of consul servers",
	}
)

func main() {
	app := cli.NewApp()
	app.Name = "munchausen"
	app.Usage = "Bootstraps a Swarm cluster with Consul"
	app.Version = "0.0.1"
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
			Usage:     "Bootstraps the cluster. A comma separated list of IPs must be passed as the first argument.",
			Flags:     []cli.Flag{flConsulServers},
			Action:    bootstrap,
		},
	}

	app.Run(os.Args)
}
