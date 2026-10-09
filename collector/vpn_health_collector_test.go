package collector

import (
	"bufio"
	"io"
	"regexp"
	"sort"
	"strings"
	"testing"

	"mikrotik-exporter/config"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
	routeros "gopkg.in/routeros.v2"
	"gopkg.in/routeros.v2/proto"
)

var vpnTestConfig = config.VPNHealth{
	Prefix: "aws_prdvpc",
	Tunnels: []config.VPNTunnel{
		{Name: "a1", Provider: "algar", StateAddress: "198.18.0.11", RemoteAddress: "18.220.90.8"},
		{Name: "a2", Provider: "algar", StateAddress: "198.18.0.12", RemoteAddress: "18.221.96.3"},
		{Name: "u1", Provider: "unifique", StateAddress: "198.18.0.13", RemoteAddress: "3.21.255.11"},
		{Name: "u2", Provider: "unifique", StateAddress: "198.18.0.14", RemoteAddress: "52.14.122.225"},
	},
}

func TestParseVPNSummary(t *testing.T) {
	s, ok := parseVPNSummary("active-A-a1")
	assert.True(t, ok)
	assert.Equal(t, vpnSummary{mode: "active", desired: "A", chosen: "a1"}, s)

	s, ok = parseVPNSummary("active-N-none")
	assert.True(t, ok)
	assert.Equal(t, "none", s.chosen)

	for _, invalid := range []string{"", "active", "active-A", "-A-a1"} {
		_, ok = parseVPNSummary(invalid)
		assert.False(t, ok, invalid)
	}
}

func TestParseVPNTunnelState(t *testing.T) {
	s, ok := parseVPNTunnelState("103")
	assert.True(t, ok)
	assert.Equal(t, vpnTunnelState{healthy: 1, failures: 0, successes: 3}, s)

	s, ok = parseVPNTunnelState("030")
	assert.True(t, ok)
	assert.Equal(t, vpnTunnelState{healthy: 0, failures: 3, successes: 0}, s)

	for _, invalid := range []string{"", "10", "1003", "1a3"} {
		_, ok = parseVPNTunnelState(invalid)
		assert.False(t, ok, invalid)
	}
}

func TestVPNHealthCollect(t *testing.T) {
	c, s := newRawPair(t)
	defer c.Close()

	go func() {
		defer s.Close()
		s.expect(t, "/system/resource/print", "=.proplist=version")
		s.writeSentence(t, "!re", "=version=6.48.2 (stable)")
		s.writeSentence(t, "!done")

		s.expect(t, "/ip/firewall/address-list/print", "=.proplist=address,comment", "?list=aws_prdvpc_health-state")
		s.writeSentence(t, "!re", "=address=198.18.0.10", "=comment=active-A-a1")
		s.writeSentence(t, "!re", "=address=198.18.0.11", "=comment=103")
		s.writeSentence(t, "!re", "=address=198.18.0.12", "=comment=103")
		s.writeSentence(t, "!re", "=address=198.18.0.13", "=comment=103")
		s.writeSentence(t, "!re", "=address=198.18.0.14", "=comment=120")
		s.writeSentence(t, "!done")

		s.expect(t, "/ip/ipsec/active-peers/print", "=.proplist=remote-address,state,uptime")
		s.writeSentence(t, "!re", "=remote-address=18.220.90.8", "=state=established", "=uptime=1h2m3s")
		s.writeSentence(t, "!re", "=remote-address=18.221.96.3", "=state=established", "=uptime=40s")
		s.writeSentence(t, "!re", "=remote-address=3.21.255.11", "=state=established", "=uptime=5m")
		s.writeSentence(t, "!done")

		s.expect(t, "/routing/bgp/advertisements/print", "=.proplist=peer,prefix")
		for _, prefix := range []string{"10.13.0.0/24", "10.12.0.0/16", "10.20.0.0/22", "10.13.0.250/32"} {
			s.writeSentence(t, "!re", "=peer=aws_prdvpc_a1_bgp", "=prefix="+prefix)
		}
		s.writeSentence(t, "!re", "=peer=aws_prdvpc_a2_bgp", "=prefix=10.13.0.251/32")
		s.writeSentence(t, "!done")

		s.expect(t, "/ip/ipsec/policy/print", "=.proplist=comment,disabled,ph2-state")
		for _, tag := range []string{"p13", "p12", "p20"} {
			s.writeSentence(t, "!re", "=comment=aws_prdvpc_a1_data-"+tag, "=disabled=false", "=ph2-state=established")
			s.writeSentence(t, "!re", "=comment=aws_prdvpc_a2_data-"+tag, "=disabled=true", "=ph2-state=no-phase2")
		}
		s.writeSentence(t, "!re", "=comment=aws_prdvpc_a1_probe-selector", "=disabled=false", "=ph2-state=established")
		s.writeSentence(t, "!done")

		s.expect(t, "/routing/bgp/peer/print", "=.proplist=name,state")
		for _, name := range []string{"a1", "a2", "u1", "u2"} {
			s.writeSentence(t, "!re", "=name=aws_prdvpc_"+name+"_bgp", "=state=established")
		}
		s.writeSentence(t, "!done")

		s.expect(t, "/routing/filter/print", "=.proplist=comment,action")
		s.writeSentence(t, "!re", "=comment=aws_prdvpc_a1_out-active", "=action=accept")
		s.writeSentence(t, "!re", "=comment=aws_prdvpc_a2_out-active", "=action=discard")
		s.writeSentence(t, "!re", "=comment=aws_prdvpc_a1_out-p12", "=action=accept")
		s.writeSentence(t, "!done")
	}()

	metrics := make(chan prometheus.Metric, 100)
	ctx := &collectorContext{ch: metrics, device: &config.Device{Name: "rb", Address: "192.0.2.1"}, client: c}
	assert.NoError(t, newVPNHealthCollector(vpnTestConfig).collect(ctx))
	close(metrics)

	got := gatherVPNMetrics(t, metrics)
	expected := map[string]float64{
		"mikrotik_vpn_monitor_up{}":                                1,
		"mikrotik_vpn_mode{desired=A,mode=active}":                 1,
		"mikrotik_vpn_tunnel_active{tunnel=a1}":                    1,
		"mikrotik_vpn_tunnel_active{tunnel=u2}":                    0,
		"mikrotik_vpn_tunnel_healthy{tunnel=u2}":                   1,
		"mikrotik_vpn_tunnel_consecutive_failures{tunnel=u2}":      2,
		"mikrotik_vpn_tunnel_consecutive_successes{tunnel=a1}":     3,
		"mikrotik_vpn_tunnel_ike_up{tunnel=a1}":                    1,
		"mikrotik_vpn_tunnel_ike_up{tunnel=u2}":                    0,
		"mikrotik_vpn_tunnel_ike_uptime_seconds{tunnel=a1}":        3723,
		"mikrotik_vpn_tunnel_bgp_up{tunnel=u1}":                    1,
		"mikrotik_vpn_tunnel_advertised_prefixes{tunnel=a1}":       4,
		"mikrotik_vpn_tunnel_advertised_prefixes{tunnel=a2}":       1,
		"mikrotik_vpn_tunnel_advertised_prefixes{tunnel=u1}":       0,
		"mikrotik_vpn_tunnel_export_accept{tunnel=a1}":             1,
		"mikrotik_vpn_tunnel_export_accept{tunnel=a2}":             0,
		"mikrotik_vpn_tunnel_data_policies_enabled{tunnel=a1}":     3,
		"mikrotik_vpn_tunnel_data_policies_established{tunnel=a1}": 3,
		"mikrotik_vpn_tunnel_data_policies_enabled{tunnel=a2}":     0,
	}
	for key, value := range expected {
		v, found := got[key]
		if assert.True(t, found, "missing %s in %v", key, got) {
			assert.Equal(t, value, v, key)
		}
	}
	_, found := got["mikrotik_vpn_tunnel_ike_uptime_seconds{tunnel=u2}"]
	assert.False(t, found, "uptime must not be exported for a tunnel without IKE")
	_, found = got["mikrotik_vpn_tunnel_export_accept{tunnel=u1}"]
	assert.False(t, found, "export metric must not be exported when the managed filter is missing")
}

func TestVPNHealthMonitorDown(t *testing.T) {
	c, s := newRawPair(t)
	defer c.Close()

	go func() {
		defer s.Close()
		s.expect(t, "/ip/firewall/address-list/print", "=.proplist=address,comment", "?list=aws_prdvpc_health-state")
		s.writeSentence(t, "!done")
	}()

	metrics := make(chan prometheus.Metric, 10)
	ctx := &collectorContext{ch: metrics, device: &config.Device{Name: "rb", Address: "192.0.2.1"}, client: c}
	collector := newVPNHealthCollector(vpnTestConfig).(*vpnHealthCollector)
	assert.NoError(t, collector.collectSchedulerState(ctx))
	close(metrics)

	got := gatherVPNMetrics(t, metrics)
	assert.Equal(t, map[string]float64{"mikrotik_vpn_monitor_up{}": 0}, got)
}

func TestVPNHealthRequiresConfig(t *testing.T) {
	ctx := &collectorContext{device: &config.Device{Name: "rb"}}
	assert.Error(t, newVPNHealthCollector(config.VPNHealth{}).collect(ctx))
}

func TestVPNHealthSkipsUnselectedDevice(t *testing.T) {
	cfg := vpnTestConfig
	cfg.Devices = []string{"other"}
	metrics := make(chan prometheus.Metric, 1)
	// No client: any RouterOS query would panic.
	ctx := &collectorContext{ch: metrics, device: &config.Device{Name: "rb"}}
	assert.NoError(t, newVPNHealthCollector(cfg).collect(ctx))
	assert.Empty(t, metrics)
}

var fqNameRegex = regexp.MustCompile(`fqName: "([^"]+)"`)

// rawServer reads raw API words: the proto reader of the library rejects query words (?key=value).
type rawServer struct {
	r *bufio.Reader
	w proto.Writer
	io.Closer
}

func newRawPair(t *testing.T) (*routeros.Client, *rawServer) {
	ar, aw := io.Pipe()
	br, bw := io.Pipe()

	c, err := routeros.NewClient(&conn{ar, bw})
	if err != nil {
		t.Fatal(err)
	}
	return c, &rawServer{bufio.NewReader(br), proto.NewWriter(aw), &conn{br, aw}}
}

func (s *rawServer) expect(t *testing.T, want ...string) {
	var got []string
	for {
		word := s.readWord(t)
		if word == "" {
			break
		}
		got = append(got, word)
	}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("sentence %q; want %q", got, want)
	}
}

func (s *rawServer) readWord(t *testing.T) string {
	first, err := s.r.ReadByte()
	if err != nil {
		t.Fatal(err)
	}
	length := int(first)
	if first&0xC0 == 0x80 {
		second, err := s.r.ReadByte()
		if err != nil {
			t.Fatal(err)
		}
		length = int(first&^0xC0)<<8 | int(second)
	} else if first&0x80 != 0 {
		t.Fatalf("unsupported word length prefix %#x", first)
	}
	b := make([]byte, length)
	if _, err := io.ReadFull(s.r, b); err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func (s *rawServer) writeSentence(t *testing.T, sentence ...string) {
	s.w.BeginSentence()
	for _, word := range sentence {
		s.w.WriteWord(word)
	}
	if err := s.w.EndSentence(); err != nil {
		t.Fatal(err)
	}
}

// gatherVPNMetrics indexes metrics by name and labels other than device name, address and provider.
func gatherVPNMetrics(t *testing.T, metrics <-chan prometheus.Metric) map[string]float64 {
	got := map[string]float64{}
	for m := range metrics {
		var pb dto.Metric
		assert.NoError(t, m.Write(&pb))
		labels := []string{}
		for _, l := range pb.Label {
			switch l.GetName() {
			case "name", "address", "provider":
			default:
				labels = append(labels, l.GetName()+"="+l.GetValue())
			}
		}
		sort.Strings(labels)
		name := fqNameRegex.FindStringSubmatch(m.Desc().String())[1]
		got[name+"{"+strings.Join(labels, ",")+"}"] = pb.Gauge.GetValue()
	}
	return got
}
