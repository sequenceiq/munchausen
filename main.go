package main

import (
	log "github.com/Sirupsen/logrus"
	"github.com/codegangsta/cli"
	"github.com/sequenceiq/swarm-bootstrap/swarmboot"
	"os"
)

func main() {
	app := cli.NewApp()
	app.Name = "munchausen"
	app.Usage = "Bootstraps a Swarm cluster with Consul"
	app.Version = "0.5.4"
	app.Author = "SequenceIQ"

	app.Flags = []cli.Flag{
		swarmboot.FlDebug,
	}

	app.Before = func(c *cli.Context) error {
		log.SetOutput(os.Stderr)
		if c.Bool(swarmboot.FlDebug.Name) {
			log.SetLevel(log.DebugLevel)
		}
		return nil
	}

	app.Commands = []cli.Command{
		{
			Name:      "bootstrap",
			ShortName: "b",
			Usage:     "Bootstraps the cluster. A comma separated list of Docker daemon addresses must be passed as the first argument.",
			Flags: []cli.Flag{swarmboot.FlConsulServers, swarmboot.FlWait,
				swarmboot.FlFallbackDNSRecursors, swarmboot.FlDockerDaemonHost, swarmboot.FlDockerDaemonPort},
			Action: swarmboot.Bootstrap,
		},
		{
			Name:      "add",
			ShortName: "a",
			Usage:     "Adds new nodes to a Consul based Swarm cluster. A comma separated list of the new Docker daemons must be passed as the first argument.",
			Flags: []cli.Flag{swarmboot.FlConsulJoin, swarmboot.FlWait, swarmboot.FlFallbackDNSRecursors,
				swarmboot.FlDockerDaemonHost, swarmboot.FlDockerDaemonPort},
			Action: swarmboot.Add,
		},
	}

	app.Run(os.Args)
}
