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

#### Notes

- *The Swarm managers (the temporary and the Consul-backed) are started on the node where the munchausen container runs (that's why the docker socket must be shared with the munchausen container)*

## Usage

The easiest way to get started is to run the docker container on one of the nodes in the cluster we'd like to bootstrap:

 ```
 docker run -it -v /var/run/docker.sock:/var/run/docker.sock \
 sequenceiq/munchausen --debug bootstrap --consulServers=<server_ip1>,<server_ip2>,... <node_ip1:2375>,<node_ip2:2375>... 
 ```

#### Notes

 - Consul server IPs are usually common internal IP addresses. `node_ips` are usually internal addresses as well but with the docker port specified.

 - The IP addresses of the consul servers must be listed in the `node_ip` list as well

 - If the Docker HTTP/HTTPS API on the machine is exposed, it can be used to bootstrap the cluster from a remote location, the only requirement is a running docker daemon on each machine.

#### Example

If you have an internal network with 5 private ip addresses: `10.0.0.1, 10.0.0.2, ... 10.0.0.5`, the docker daemon is configured to listen on port 2375 (`-H=0.0.0.0:2375`) on every machine and you'd like to have the first 3 nodes as consul servers, the bootstrap command should look like this:

 ```
 docker run -it -v /var/run/docker.sock:/var/run/docker.sock \
 sequenceiq/munchausen --debug bootstrap --consulServers=10.0.0.1,10.0.0.2,10.0.0.3 10.0.0.1:2375,10.0.0.2:2375,10.0.0.3:2375,10.0.0.4:2375,10.0.0.5:2375
 ```

### Adding nodes

 New nodes can be added to an existing Consul-Swarm cluster with the `add` command instead of `bootstrap`. Its usage is quite similar, there is only one difference - the `add` command doesn't create a new Consul cluster with new Consul servers, rather joins an existing one and adds only Consul agents. That's why the `--consulServers` switch is replaced with `--join`:


 ```
 docker run -it -v /var/run/docker.sock:/var/run/docker.sock \
 sequenceiq/munchausen --debug add --join=consul://<consul_ip>:8500 <node_ip1:2375>,<node_ip2:2375>... 
 ```

#### Example

 If you'd like to add `10.0.0.6` and `10.0.0.7` to the cluster above and configured the docker daemon to listen on port 2375 on these 2 instances, run the following command:

```
 docker run -it -v /var/run/docker.sock:/var/run/docker.sock \
 sequenceiq/munchausen --debug bootstrap --join=consul://10.0.0.1:8500 10.0.0.6:2375,10.0.0.7:2375
 ```


## Debug

Use the `--debug` switch
```
munchausen --debug bootstrap --consulServers=<server_ip1>,<server_ip2>,... <node_ip1:2375>,<node_ip2:2375>... 
```

or provide an environment variable:
```
DEBUG=1 munchausen bootstrap --consulServers=<server_ip1>,<server_ip2>,... <node_ip1:2375>,<node_ip2:2375>... 
```

## Dependency management

Munchausen uses [Godep](https://github.com/tools/godep) for dependency management. You can install it and download the configured dependency versions with:
```
go get github.com/tools/godep
godep restore
```

## Notes

This is a work in progress, the following things will be added soon:

- support Swarm with TLS

- bootstrapping from a static file descriptor