package collector

import (
	"fmt"
	"strconv"
	"strings"

	"mikrotik-exporter/config"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
	"gopkg.in/routeros.v2/proto"
)

const vpnHealthDefaultSummaryAddress = "198.18.0.10"

type vpnHealthCollector struct {
	cfg config.VPNHealth

	monitorUp           *prometheus.Desc
	mode                *prometheus.Desc
	active              *prometheus.Desc
	healthy             *prometheus.Desc
	failures            *prometheus.Desc
	successes           *prometheus.Desc
	ikeUp               *prometheus.Desc
	ikeUptime           *prometheus.Desc
	bgpUp               *prometheus.Desc
	advertised          *prometheus.Desc
	exportAccept        *prometheus.Desc
	policiesEnabled     *prometheus.Desc
	policiesEstablished *prometheus.Desc
}

// vpnTunnelState is the per-tunnel state written by the scheduler: <healthy><failures><successes>.
type vpnTunnelState struct {
	healthy, failures, successes float64
}

// vpnSummary is the scheduler summary: <mode>-<A|U|N>-<chosen tunnel|none>.
type vpnSummary struct {
	mode, desired, chosen string
}

func newVPNHealthCollector(cfg config.VPNHealth) routerOSCollector {
	if cfg.SummaryAddress == "" {
		cfg.SummaryAddress = vpnHealthDefaultSummaryAddress
	}
	c := &vpnHealthCollector{cfg: cfg}
	c.init()
	return c
}

func (c *vpnHealthCollector) init() {
	const prefix = "vpn"
	device := []string{"name", "address"}
	tunnel := []string{"name", "address", "tunnel", "provider"}

	c.monitorUp = description(prefix, "monitor_up", "health scheduler state present on the device (1 = present)", device)
	c.mode = description(prefix, "mode", "health scheduler mode and desired provider (A, U or N), value is always 1", append(device, "mode", "desired"))
	c.active = description(prefix, "tunnel_active", "tunnel chosen by the health scheduler to carry data (1 = active)", tunnel)
	c.healthy = description(prefix, "tunnel_healthy", "data probe health after hysteresis (1 = healthy)", tunnel)
	c.failures = description(prefix, "tunnel_consecutive_failures", "consecutive failed data probes (capped at the hysteresis threshold)", tunnel)
	c.successes = description(prefix, "tunnel_consecutive_successes", "consecutive successful data probes (capped at the hysteresis threshold)", tunnel)
	c.ikeUp = description(prefix, "tunnel_ike_up", "IKE phase 1 established with the tunnel remote address (1 = established)", tunnel)
	c.ikeUptime = description(prefix, "tunnel_ike_uptime_seconds", "IKE phase 1 uptime in seconds", tunnel)
	c.bgpUp = description(prefix, "tunnel_bgp_up", "BGP session of the tunnel is established (1 = established)", tunnel)
	c.advertised = description(prefix, "tunnel_advertised_prefixes", "prefixes advertised to the BGP peer of the tunnel", tunnel)
	c.exportAccept = description(prefix, "tunnel_export_accept", "managed export filter accepts the aggregates (1 = accept)", tunnel)
	c.policiesEnabled = description(prefix, "tunnel_data_policies_enabled", "enabled IPsec data policies of the tunnel", tunnel)
	c.policiesEstablished = description(prefix, "tunnel_data_policies_established", "IPsec data policies of the tunnel with phase 2 established", tunnel)
}

func (c *vpnHealthCollector) describe(ch chan<- *prometheus.Desc) {
	for _, d := range []*prometheus.Desc{
		c.monitorUp, c.mode, c.active, c.healthy, c.failures, c.successes, c.ikeUp, c.ikeUptime,
		c.bgpUp, c.advertised, c.exportAccept, c.policiesEnabled, c.policiesEstablished,
	} {
		ch <- d
	}
}

func (c *vpnHealthCollector) collect(ctx *collectorContext) error {
	if c.cfg.Prefix == "" || len(c.cfg.Tunnels) == 0 {
		return fmt.Errorf("vpn_health: prefix and tunnels must be configured")
	}
	if !c.appliesTo(ctx.device.Name) {
		return nil
	}

	// Each source is optional: one failing query must not hide the others.
	var lastErr error
	steps := []func(*collectorContext) error{c.collectSchedulerState, c.collectIKE, c.collectAdvertisements, c.collectDataPolicies}
	if major, err := routerOSMajor(ctx); err != nil {
		log.WithFields(log.Fields{"device": ctx.device.Name, "error": err}).Error("error fetching RouterOS version")
		lastErr = err
	} else {
		steps = append(steps,
			func(ctx *collectorContext) error { return c.collectBGP(ctx, major) },
			func(ctx *collectorContext) error { return c.collectExportFilters(ctx, major) },
		)
	}
	for _, step := range steps {
		if err := step(ctx); err != nil {
			log.WithFields(log.Fields{"device": ctx.device.Name, "error": err}).Error("error fetching vpn health metrics")
			lastErr = err
		}
	}
	return lastErr
}

func (c *vpnHealthCollector) run(ctx *collectorContext, command string, query []string, props ...string) ([]*proto.Sentence, error) {
	args := append([]string{command, "=.proplist=" + strings.Join(props, ",")}, query...)
	reply, err := ctx.client.RunArgs(args)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", command, err)
	}
	return reply.Re, nil
}

func (c *vpnHealthCollector) send(ctx *collectorContext, desc *prometheus.Desc, value float64, t config.VPNTunnel) {
	ctx.ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, value, ctx.device.Name, ctx.device.Address, t.Name, t.Provider)
}

func (c *vpnHealthCollector) collectSchedulerState(ctx *collectorContext) error {
	stateList := c.cfg.Prefix + "_health-state"
	entries, err := c.run(ctx, "/ip/firewall/address-list/print", []string{"?list=" + stateList}, "address", "comment")
	if err != nil {
		return err
	}
	comments := map[string]string{}
	for _, re := range entries {
		comments[re.Map["address"]] = re.Map["comment"]
	}

	summary, ok := parseVPNSummary(comments[c.cfg.SummaryAddress])
	if !ok {
		ctx.ch <- prometheus.MustNewConstMetric(c.monitorUp, prometheus.GaugeValue, 0, ctx.device.Name, ctx.device.Address)
		return nil
	}
	ctx.ch <- prometheus.MustNewConstMetric(c.monitorUp, prometheus.GaugeValue, 1, ctx.device.Name, ctx.device.Address)
	ctx.ch <- prometheus.MustNewConstMetric(c.mode, prometheus.GaugeValue, 1, ctx.device.Name, ctx.device.Address, summary.mode, summary.desired)

	for _, t := range c.cfg.Tunnels {
		c.send(ctx, c.active, boolToFloat(summary.chosen == t.Name), t)
		state, ok := parseVPNTunnelState(comments[t.StateAddress])
		if !ok {
			continue
		}
		c.send(ctx, c.healthy, state.healthy, t)
		c.send(ctx, c.failures, state.failures, t)
		c.send(ctx, c.successes, state.successes, t)
	}
	return nil
}

func (c *vpnHealthCollector) collectIKE(ctx *collectorContext) error {
	peers, err := c.run(ctx, "/ip/ipsec/active-peers/print", nil, "remote-address", "state", "uptime")
	if err != nil {
		return err
	}
	byRemote := map[string]*proto.Sentence{}
	for _, re := range peers {
		byRemote[re.Map["remote-address"]] = re
	}

	for _, t := range c.cfg.Tunnels {
		if t.RemoteAddress == "" {
			continue
		}
		re, found := byRemote[t.RemoteAddress]
		up := found && re.Map["state"] == "established"
		c.send(ctx, c.ikeUp, boolToFloat(up), t)
		if up {
			if uptime, err := parseDuration(re.Map["uptime"]); err == nil {
				c.send(ctx, c.ikeUptime, uptime, t)
			}
		}
	}
	return nil
}

func (c *vpnHealthCollector) collectBGP(ctx *collectorContext, major int) error {
	up := map[string]bool{}
	if major >= 7 {
		var err error
		if up, err = c.bgpUpV7(ctx); err != nil {
			return err
		}
	} else {
		peers, err := c.run(ctx, "/routing/bgp/peer/print", nil, "name", "state")
		if err != nil {
			return err
		}
		for _, re := range peers {
			up[re.Map["name"]] = re.Map["state"] == "established"
		}
	}
	for _, t := range c.cfg.Tunnels {
		if established, found := up[c.objectName(t, "bgp")]; found {
			c.send(ctx, c.bgpUp, boolToFloat(established), t)
		}
	}
	return nil
}

// bgpUpV7 lists connections (sessions may be absent while down) and marks those with an established session.
func (c *vpnHealthCollector) bgpUpV7(ctx *collectorContext) (map[string]bool, error) {
	connections, err := c.run(ctx, "/routing/bgp/connection/print", nil, "name")
	if err != nil {
		return nil, err
	}
	sessions, err := c.run(ctx, "/routing/bgp/session/print", nil, "name", "established")
	if err != nil {
		return nil, err
	}
	up := map[string]bool{}
	for _, re := range connections {
		up[re.Map["name"]] = false
	}
	for _, re := range sessions {
		if re.Map["established"] == "true" {
			up[sessionConnection(re.Map["name"])] = true
		}
	}
	return up, nil
}

func (c *vpnHealthCollector) collectAdvertisements(ctx *collectorContext) error {
	advertisements, err := c.run(ctx, "/routing/bgp/advertisements/print", nil, "peer", "prefix")
	if err != nil {
		return err
	}
	counts := map[string]float64{}
	for _, re := range advertisements {
		counts[sessionConnection(re.Map["peer"])]++
	}
	for _, t := range c.cfg.Tunnels {
		c.send(ctx, c.advertised, counts[c.objectName(t, "bgp")], t)
	}
	return nil
}

func (c *vpnHealthCollector) collectExportFilters(ctx *collectorContext, major int) error {
	actions := map[string]string{}
	if major >= 7 {
		rules, err := c.run(ctx, "/routing/filter/rule/print", nil, "comment", "rule")
		if err != nil {
			return err
		}
		for _, re := range rules {
			actions[re.Map["comment"]] = filterRuleAction(re.Map["rule"])
		}
	} else {
		filters, err := c.run(ctx, "/routing/filter/print", nil, "comment", "action")
		if err != nil {
			return err
		}
		for _, re := range filters {
			actions[re.Map["comment"]] = re.Map["action"]
		}
	}
	for _, t := range c.cfg.Tunnels {
		if action, found := actions[c.objectName(t, "out-active")]; found {
			c.send(ctx, c.exportAccept, boolToFloat(action == "accept"), t)
		}
	}
	return nil
}

func (c *vpnHealthCollector) collectDataPolicies(ctx *collectorContext) error {
	policies, err := c.run(ctx, "/ip/ipsec/policy/print", nil, "comment", "disabled", "ph2-state")
	if err != nil {
		return err
	}
	for _, t := range c.cfg.Tunnels {
		var enabled, established float64
		prefix := c.objectName(t, "data-")
		for _, re := range policies {
			if !strings.HasPrefix(re.Map["comment"], prefix) || re.Map["disabled"] == "true" {
				continue
			}
			enabled++
			if re.Map["ph2-state"] == "established" {
				established++
			}
		}
		c.send(ctx, c.policiesEnabled, enabled, t)
		c.send(ctx, c.policiesEstablished, established, t)
	}
	return nil
}

// appliesTo reports whether the device is selected; an empty device list selects all devices.
func (c *vpnHealthCollector) appliesTo(device string) bool {
	if len(c.cfg.Devices) == 0 {
		return true
	}
	for _, name := range c.cfg.Devices {
		if name == device {
			return true
		}
	}
	return false
}

// objectName follows the RouterOS naming convention <prefix>_<tunnel>_<suffix>.
func (c *vpnHealthCollector) objectName(t config.VPNTunnel, suffix string) string {
	return c.cfg.Prefix + "_" + t.Name + "_" + suffix
}

func parseVPNSummary(comment string) (vpnSummary, bool) {
	parts := strings.SplitN(comment, "-", 3)
	if len(parts) != 3 || parts[0] == "" || parts[2] == "" {
		return vpnSummary{}, false
	}
	return vpnSummary{mode: parts[0], desired: parts[1], chosen: parts[2]}, true
}

func parseVPNTunnelState(comment string) (vpnTunnelState, bool) {
	if len(comment) != 3 {
		return vpnTunnelState{}, false
	}
	var digits [3]float64
	for i := range digits {
		v, err := strconv.Atoi(comment[i : i+1])
		if err != nil {
			return vpnTunnelState{}, false
		}
		digits[i] = float64(v)
	}
	return vpnTunnelState{healthy: digits[0], failures: digits[1], successes: digits[2]}, true
}

func boolToFloat(b bool) float64 {
	if b {
		return 1
	}
	return 0
}
