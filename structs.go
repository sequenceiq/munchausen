package main

import ()

const (
	ConsulImage           = "sequenceiq/consul:v0.4.1.ptr"
	SwarmImage            = "swarm"
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

type ConsulConfig struct {
	BootstrapExpect    int        `json:"bootstrap_expect"`
	Server             bool       `json:"server"`
	AdvertiseAddr      string     `json:"advertise_addr,omitempty"`
	DataDir            string     `json:"data_dir"`
	UiDir              string     `json:"ui_dir"`
	ClientAddr         string     `json:"client_addr"`
	DNSRecursor        string     `json:"recursor"`
	DisableUpdateCheck bool       `json:"disable_update_check"`
	RetryJoin          []string   `json:"retry_join"`
	EncryptKey         string     `json:"encrypt,omitempty"`
	VerifyIncoming     bool       `json:"verify_incoming,omitempty",`
	VerifyOutgoing     bool       `json:"verify_outgoing,omitempty"`
	CAFile             string     `json:"ca_file,omitempty"`
	CertFile           string     `json:"cert_file,omitempty"`
	KeyFile            string     `json:"key_file,omitempty"`
	Ports              PortConfig `json:"ports"`
}
