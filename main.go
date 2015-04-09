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
		Name:  "consul",
		Value: "",
		Usage: "The consul cluster to join (consul://<consul_addr>:<port>)",
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
			Usage:     "Bootstraps the cluster. A comma separated list of Docker daemon addresses must be passed as the first argument.",
			Flags:     []cli.Flag{flConsulServers},
			Action:    bootstrap,
		},
		{
			Name:      "add",
			ShortName: "a",
			Usage:     "Adds new nodes to a Consul based Swarm cluster. A comma separated list of the new Docker daemons must be passed as the first argument.",
			Flags:     []cli.Flag{flConsulJoin},
			Action:    add,
		},
	}

	app.Run(os.Args)
}
