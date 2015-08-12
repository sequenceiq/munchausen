package main

import ()

const (
	ConsulImage           = "sequenceiq/consul:v0.5.0-v5"
	SwarmImage            = "swarm:0.4.0"
	TmpSwarmContainerName = "tmp-swarm-manager"
	SwarmContainerName    = "swarm-manager"
)

const (
	Server = iota
	Agent
)

type empty struct{}
type ConsulNode int

type PortConfig struct {
	DNS   int
	HTTP  int
	HTTPS int
}

type DNSConfig struct {
	AllowStale  bool              `json:"allow_stale"`
	MaxStale    string            `json:"max_stale"`
	NodeTTL     string            `json:"node_ttl"`
}

type ConsulConfig struct {
	BootstrapExpect    int        `json:"bootstrap_expect"`
	Server             bool       `json:"server"`
	AdvertiseAddr      string     `json:"advertise_addr,omitempty"`
	DataDir            string     `json:"data_dir"`
	UiDir              string     `json:"ui_dir"`
	ClientAddr         string     `json:"client_addr"`
	DNSRecursors       []string   `json:"recursors"`
	DisableUpdateCheck bool       `json:"disable_update_check"`
	RetryJoin          []string   `json:"retry_join"`
	EncryptKey         string     `json:"encrypt,omitempty"`
	VerifyIncoming     bool       `json:"verify_incoming,omitempty",`
	VerifyOutgoing     bool       `json:"verify_outgoing,omitempty"`
	CAFile             string     `json:"ca_file,omitempty"`
	CertFile           string     `json:"cert_file,omitempty"`
	KeyFile            string     `json:"key_file,omitempty"`
	Ports              PortConfig `json:"ports"`
	DNS                DNSConfig  `json:"dns_config"`
}

type SwarmNode struct {
	ID     string
	IP     string
	Addr   string
	Name   string
	CPUs   int64
	Memory int64
	Labels map[string]string
}
