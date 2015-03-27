Munchausen is a tool to bootstrap a Docker Swarm cluster that uses Consul as the discovery backend.

## Concept

Swarm can be started easily with a Consul based discovery backend after the Consul cluster is already up and running.

But usually Consul agents (as well as Swarm agents) are also running in docker containers so these containers must be started on all nodes first.

So we need a lot of containers running on different nodes - that's what Swarm is for, but we cannot use that because the Consul cluster is not ready yet.

This looks like a chicken-and-egg problem, and this tool is supposed to solve it.

## Steps
    
- A temporary Swarm manager is started on the target machine with a static list of IPs. This discovery method has the advantage that no Swarm agents are needed on the other nodes, but it can't handle node failures or additions automatically.

- The Swarm manager is used to start the Consul containers on every node in the cluster (with a configurable amount of servers).

- Swarm agents with a Consul address are started on each node by the Swarm manager.

- The Consul-based Swarm manager is started on the same instance where the temporary manager was started.

- The temporary Swarm manager is removed.

## Usage

The easiest way to get started is to run the docker container on one of the nodes in the cluster we'd like to bootstrap:

 ```
 docker run -it -v /var/run/docker.sock:/var/run/docker.sock \
 sequenceiq/munchausen bootstrap --consulServers=3 <node_ip1:2375>,<node_ip2:2375>... 
 ```

 *If the Docker HTTP/HTTPS API on the machine is exposed, it can be used to bootstrap the cluster from a remote location, the only requirement is a running docker daemon on each machine.*

## Debug

Use the `--debug` switch
```
munchausen --debug bootstrap --consulServers=3 <node_ip1:2375>,<node_ip2:2375>... 
```

or provide an environment variable:
```
DEBUG=1 munchausen bootstrap --consulServers=3 <node_ip1:2375>,<node_ip2:2375>... 
```

## Notes

This is a work in progress, the following things will be added soon:

- bootstrapping from a static file descriptor

- support Swarm with TLS

- explicitly setting which nodes are Consul servers

- error handling