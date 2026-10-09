package collector

import (
	"testing"

	"mikrotik-exporter/config"

	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
	"github.com/stretchr/testify/assert"
)

func TestRouterOSV7Helpers(t *testing.T) {
	assert.Equal(t, "aws_prdvpc_a1_bgp", sessionConnection("aws_prdvpc_a1_bgp-1"))
	assert.Equal(t, "aws_prdvpc_a1_bgp", sessionConnection("aws_prdvpc_a1_bgp"))
	assert.Equal(t, "peer-2", sessionConnection("peer-2-1"))

	assert.Equal(t, "accept", filterRuleAction("if (dst in 10.0.0.0/8 && dst-len in 16-24) { accept; }"))
	assert.Equal(t, "reject", filterRuleAction("if (dst in 10.0.0.0/8 && dst-len in 16-24) { reject; }"))
	assert.Equal(t, "reject", filterRuleAction("{ reject; }"))
	assert.Equal(t, "", filterRuleAction("if (dst in 172.30.0.0/16) { set bgp-local-pref 100; accept; }"))
}

// RouterOS v7 sample captured from the CHR 7.24.5 lab (converted production config).
func TestVPNHealthCollectV7(t *testing.T) {
	c, s := newRawPair(t)
	defer c.Close()

	go func() {
		defer s.Close()
		s.expect(t, "/routing/bgp/connection/print", "=.proplist=name")
		for _, name := range []string{"a1", "a2", "u1", "u2"} {
			s.writeSentence(t, "!re", "=name=aws_prdvpc_"+name+"_bgp")
		}
		s.writeSentence(t, "!done")
		s.expect(t, "/routing/bgp/session/print", "=.proplist=name,established")
		s.writeSentence(t, "!re", "=name=aws_prdvpc_a1_bgp-1", "=established=true")
		s.writeSentence(t, "!re", "=name=aws_prdvpc_u1_bgp-1", "=established=false")
		s.writeSentence(t, "!done")

		s.expect(t, "/routing/bgp/advertisements/print", "=.proplist=peer,prefix")
		for range 4 {
			s.writeSentence(t, "!re", "=peer=aws_prdvpc_a1_bgp-1")
		}
		s.writeSentence(t, "!re", "=peer=aws_prdvpc_a2_bgp-1")
		s.writeSentence(t, "!done")

		s.expect(t, "/routing/filter/rule/print", "=.proplist=comment,rule")
		s.writeSentence(t, "!re", "=comment=aws_prdvpc_a1_out-active", "=rule=if (dst in 10.0.0.0/8 && dst-len in 16-24) { accept; }")
		s.writeSentence(t, "!re", "=comment=aws_prdvpc_a2_out-active", "=rule=if (dst in 10.0.0.0/8 && dst-len in 16-24) { reject; }")
		s.writeSentence(t, "!done")
	}()

	metrics := make(chan prometheus.Metric, 100)
	ctx := &collectorContext{ch: metrics, device: &config.Device{Name: "rb", Address: "192.0.2.1"}, client: c}
	collector := newVPNHealthCollector(vpnTestConfig).(*vpnHealthCollector)
	assert.NoError(t, collector.collectBGP(ctx, 7))
	assert.NoError(t, collector.collectAdvertisements(ctx))
	assert.NoError(t, collector.collectExportFilters(ctx, 7))
	close(metrics)

	got := gatherVPNMetrics(t, metrics)
	expected := map[string]float64{
		"mikrotik_vpn_tunnel_bgp_up{tunnel=a1}":              1,
		"mikrotik_vpn_tunnel_bgp_up{tunnel=u1}":              0,
		"mikrotik_vpn_tunnel_bgp_up{tunnel=u2}":              0,
		"mikrotik_vpn_tunnel_advertised_prefixes{tunnel=a1}": 4,
		"mikrotik_vpn_tunnel_advertised_prefixes{tunnel=a2}": 1,
		"mikrotik_vpn_tunnel_advertised_prefixes{tunnel=u1}": 0,
		"mikrotik_vpn_tunnel_export_accept{tunnel=a1}":       1,
		"mikrotik_vpn_tunnel_export_accept{tunnel=a2}":       0,
	}
	for key, value := range expected {
		v, found := got[key]
		if assert.True(t, found, "missing %s in %v", key, got) {
			assert.Equal(t, value, v, key)
		}
	}
}

func TestBGPCollectV7(t *testing.T) {
	c, s := newRawPair(t)
	defer c.Close()

	go func() {
		defer s.Close()
		s.expect(t, "/system/resource/print", "=.proplist=version")
		s.writeSentence(t, "!re", "=version=7.24.5 (stable)")
		s.writeSentence(t, "!done")
		s.expect(t, "/routing/bgp/connection/print", "=.proplist=name,remote.as")
		s.writeSentence(t, "!re", "=name=aws_prdvpc_a1_bgp", "=remote.as=64512")
		s.writeSentence(t, "!re", "=name=aws_prdvpc_u2_bgp", "=remote.as=64512")
		s.writeSentence(t, "!done")
		s.expect(t, "/routing/bgp/session/print", "=.proplist=name,remote.as,established,prefix-count")
		s.writeSentence(t, "!re", "=name=aws_prdvpc_a1_bgp-1", "=remote.as=64512", "=established=true", "=prefix-count=1")
		s.writeSentence(t, "!done")
	}()

	metrics := make(chan prometheus.Metric, 10)
	ctx := &collectorContext{ch: metrics, device: &config.Device{Name: "rb", Address: "192.0.2.1"}, client: c}
	assert.NoError(t, newBGPCollector().collect(ctx))
	close(metrics)

	got := map[string]float64{}
	for m := range metrics {
		var pb dto.Metric
		assert.NoError(t, m.Write(&pb))
		session := ""
		for _, l := range pb.GetLabel() {
			if l.GetName() == "session" {
				session = l.GetValue()
			}
		}
		got[fqNameRegex.FindStringSubmatch(m.Desc().String())[1]+"{"+session+"}"] = pb.GetGauge().GetValue()
	}
	assert.Equal(t, map[string]float64{
		"mikrotik_bgp_up{aws_prdvpc_a1_bgp}":           1,
		"mikrotik_bgp_prefix_count{aws_prdvpc_a1_bgp}": 1,
		"mikrotik_bgp_up{aws_prdvpc_u2_bgp}":           0,
		"mikrotik_bgp_prefix_count{aws_prdvpc_u2_bgp}": 0,
	}, got)
}
