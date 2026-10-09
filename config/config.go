package config

import (
	"io"
	"io/ioutil"

	yaml "gopkg.in/yaml.v2"
)

// Config represents the configuration for the exporter
type Config struct {
	Devices  []Device `yaml:"devices"`
	Features struct {
		BGP          bool `yaml:"bgp,omitempty"`
		Conntrack    bool `yaml:"conntrack,omitempty"`
		DHCP         bool `yaml:"dhcp,omitempty"`
		DHCPL        bool `yaml:"dhcpl,omitempty"`
		DHCPv6       bool `yaml:"dhcpv6,omitempty"`
		Firmware     bool `yaml:"firmware,omitempty"`
		Health       bool `yaml:"health,omitempty"`
		Routes       bool `yaml:"routes,omitempty"`
		RoutesDetail bool `yaml:"routes_detail,omitempty"`
		POE          bool `yaml:"poe,omitempty"`
		Pools        bool `yaml:"pools,omitempty"`
		Optics       bool `yaml:"optics,omitempty"`
		W60G         bool `yaml:"w60g,omitempty"`
		WlanSTA      bool `yaml:"wlansta,omitempty"`
		Capsman      bool `yaml:"capsman,omitempty"`
		WlanIF       bool `yaml:"wlanif,omitempty"`
		Monitor      bool `yaml:"monitor,omitempty"`
		Ipsec        bool `yaml:"ipsec,omitempty"`
		Lte          bool `yaml:"lte,omitempty"`
		Netwatch     bool `yaml:"netwatch,omitempty"`
		Cloud        bool `yaml:"cloud,omitempty"`
		VPNHealth    bool `yaml:"vpn_health,omitempty"`
	} `yaml:"features,omitempty"`
	VPNHealth VPNHealth `yaml:"vpn_health,omitempty"`
}

// VPNHealth describes the tunnels watched by a RouterOS health scheduler.
// Object names are derived from Prefix: <prefix>_health-state, <prefix>_<tunnel>_bgp,
// <prefix>_<tunnel>_out-active and <prefix>_<tunnel>_data-*.
type VPNHealth struct {
	Prefix         string      `yaml:"prefix"`
	SummaryAddress string      `yaml:"summary_address"`
	Devices        []string    `yaml:"devices"`
	Tunnels        []VPNTunnel `yaml:"tunnels"`
}

// VPNTunnel identifies one tunnel watched by the health scheduler
type VPNTunnel struct {
	Name          string `yaml:"name"`
	Provider      string `yaml:"provider"`
	StateAddress  string `yaml:"state_address"`
	RemoteAddress string `yaml:"remote_address"`
}

// Device represents a target device
type Device struct {
	Name      string    `yaml:"name"`
	Address   string    `yaml:"address,omitempty"`
	Srv       SrvRecord `yaml:"srv,omitempty"`
	User      string    `yaml:"user"`
	Password  string    `yaml:"password"`
	Port      string    `yaml:"port"`
	Wifiwave2 bool      `yaml:"wifiwave2"`
}

type SrvRecord struct {
	Record string    `yaml:"record"`
	Dns    DnsServer `yaml:"dns,omitempty"`
}
type DnsServer struct {
	Address string `yaml:"address"`
	Port    int    `yaml:"port"`
}

// Load reads YAML from reader and unmashals in Config
func Load(r io.Reader) (*Config, error) {
	b, err := ioutil.ReadAll(r)
	if err != nil {
		return nil, err
	}

	c := &Config{}
	err = yaml.Unmarshal(b, c)
	if err != nil {
		return nil, err
	}

	return c, nil
}
