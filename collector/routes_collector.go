package collector

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/prometheus/client_golang/prometheus"
	log "github.com/sirupsen/logrus"
)

type routesCollector struct {
	protocols         []string
	detailed          bool
	countDesc         *prometheus.Desc
	countProtocolDesc *prometheus.Desc
	routeInfoDesc     *prometheus.Desc
	routeStatusDesc   *prometheus.Desc
	routeDistanceDesc *prometheus.Desc
	routeScopeDesc    *prometheus.Desc
	routeTargetDesc   *prometheus.Desc
}

func newRoutesCollector(detailed bool) routerOSCollector {
	c := &routesCollector{detailed: detailed}
	c.init()
	return c
}

func (c *routesCollector) init() {
	const prefix = "routes"
	labelNames := []string{"name", "address", "ip_version"}
	c.countDesc = description(prefix, "total_count", "number of routes in RIB", labelNames)
	c.countProtocolDesc = description(prefix, "protocol_count", "number of routes per protocol in RIB", append(labelNames, "protocol"))
	if c.detailed {
		routeInfoLabels := []string{"name", "address", "ip_version", "dst_address", "gateway", "comment", "distance", "active", "static", "connected", "enabled", "gateway_reachable"}
		routeValueLabels := []string{"name", "address", "ip_version", "dst_address", "gateway", "comment", "distance", "active", "static", "connected", "enabled", "gateway_reachable"}
		c.routeInfoDesc = description(prefix, "info", "information about non-BGP routes", routeInfoLabels)
		c.routeStatusDesc = description(prefix, "protocol_status_count", "number of routes per protocol and status", []string{"name", "address", "ip_version", "protocol", "status"})
		c.routeDistanceDesc = description(prefix, "distance", "route distance", routeValueLabels)
		c.routeScopeDesc = description(prefix, "scope", "route scope", routeValueLabels)
		c.routeTargetDesc = description(prefix, "target_scope", "route target scope", routeValueLabels)
	}

	c.protocols = []string{"bgp", "static", "ospf", "dynamic", "connect", "rip"}
}

func (c *routesCollector) describe(ch chan<- *prometheus.Desc) {
	ch <- c.countDesc
	ch <- c.countProtocolDesc
	if c.detailed {
		ch <- c.routeInfoDesc
		ch <- c.routeStatusDesc
		ch <- c.routeDistanceDesc
		ch <- c.routeScopeDesc
		ch <- c.routeTargetDesc
	}
}

func (c *routesCollector) collect(ctx *collectorContext) error {
	err := c.colllectForIPVersion("4", "ip", ctx)
	if err != nil {
		return err
	}

	return c.colllectForIPVersion("6", "ipv6", ctx)
}

func (c *routesCollector) colllectForIPVersion(ipVersion, topic string, ctx *collectorContext) error {
	err := c.colllectCount(ipVersion, topic, ctx)
	if err != nil {
		return err
	}

	for _, p := range c.protocols {
		err := c.colllectCountProtcol(ipVersion, topic, p, ctx)
		if err != nil {
			return err
		}
	}

	if c.detailed {
		return c.collectDetails(ipVersion, topic, ctx)
	}

	return nil
}

func (c *routesCollector) collectDetails(ipVersion, topic string, ctx *collectorContext) error {
	reply, err := ctx.client.Run(fmt.Sprintf("/%s/route/print", topic), "=detail=")
	if err != nil {
		log.WithFields(log.Fields{
			"ip_version": ipVersion,
			"device":     ctx.device.Name,
			"topic":      topic,
			"error":      err,
		}).Error("error fetching detailed routes metrics")
		return err
	}

	bgpStatuses := make(map[string]float64)
	for _, route := range reply.Re {
		protocol := routeProtocol(route.Map)
		status := routeStatus(route.Map)
		if protocol == "bgp" {
			bgpStatuses[status]++
			continue
		}

		infoLabels := []string{
			ctx.device.Name,
			ctx.device.Address,
			ipVersion,
			cleanLabelValue(route.Map["dst-address"]),
			cleanLabelValue(route.Map["gateway"]),
			cleanLabelValue(route.Map["comment"]),
			route.Map["distance"],
			routeFlagLabel(route.Map, "active"),
			routeFlagLabel(route.Map, "static"),
			routeFlagLabel(route.Map, "connected", "connect"),
			routeEnabledLabel(route.Map),
			gatewayReachableLabel(route.Map),
		}
		ctx.ch <- prometheus.MustNewConstMetric(c.routeInfoDesc, prometheus.GaugeValue, 1, infoLabels...)
		c.collectNumericRouteMetric(c.routeDistanceDesc, route.Map["distance"], infoLabels, ctx)
		c.collectNumericRouteMetric(c.routeScopeDesc, route.Map["scope"], infoLabels, ctx)
		c.collectNumericRouteMetric(c.routeTargetDesc, route.Map["target-scope"], infoLabels, ctx)
	}
	for status, count := range bgpStatuses {
		ctx.ch <- prometheus.MustNewConstMetric(
			c.routeStatusDesc,
			prometheus.GaugeValue,
			count,
			ctx.device.Name,
			ctx.device.Address,
			ipVersion,
			"bgp",
			status,
		)
	}

	return nil
}

func (c *routesCollector) collectNumericRouteMetric(desc *prometheus.Desc, value string, labels []string, ctx *collectorContext) {
	if value == "" {
		return
	}
	v, err := strconv.ParseFloat(value, 64)
	if err != nil {
		log.WithFields(log.Fields{
			"value":  value,
			"device": ctx.device.Name,
			"error":  err,
		}).Warn("error parsing route metric")
		return
	}
	ctx.ch <- prometheus.MustNewConstMetric(desc, prometheus.GaugeValue, v, labels...)
}

func routeProtocol(values map[string]string) string {
	if protocol := values["protocol"]; protocol != "" {
		if protocol == "connected" {
			return "connect"
		}
		return protocol
	}
	for _, protocol := range []string{"bgp", "ospf", "rip", "static", "connect", "dynamic"} {
		if routeFlag(values, protocol) {
			return protocol
		}
	}
	return "unknown"
}

func routeStatus(values map[string]string) string {
	switch {
	case routeFlag(values, "disabled"):
		return "disabled"
	case routeFlag(values, "active"):
		return "active"
	case values["active"] == "no" || values["active"] == "false" || routeFlag(values, "inactive"):
		return "inactive"
	default:
		return "unknown"
	}
}

func routeFlag(values map[string]string, names ...string) bool {
	for _, name := range names {
		value, ok := values[name]
		if !ok {
			continue
		}
		return value == "yes" || value == "true"
	}
	return false
}

func routeFlagLabel(values map[string]string, names ...string) string {
	if routeFlag(values, names...) {
		return "true"
	}
	return "false"
}

func routeEnabledLabel(values map[string]string) string {
	if value, ok := values["enabled"]; ok {
		return routeFlagLabel(map[string]string{"enabled": value}, "enabled")
	}
	if routeFlag(values, "disabled") {
		return "false"
	}
	return "true"
}

func gatewayReachableLabel(values map[string]string) string {
	status := strings.ToLower(values["gateway-status"])
	switch {
	case strings.Contains(status, "unreachable"), strings.Contains(status, "inactive"):
		return "false"
	case strings.Contains(status, "reachable"):
		return "true"
	default:
		return "false"
	}
}

func (c *routesCollector) colllectCount(ipVersion, topic string, ctx *collectorContext) error {
	reply, err := ctx.client.Run(fmt.Sprintf("/%s/route/print", topic), "?disabled=false", "=count-only=")
	if err != nil {
		log.WithFields(log.Fields{
			"ip_version": ipVersion,
			"device":     ctx.device.Name,
			"topic":      topic,
			"error":      err,
		}).Error("error fetching routes metrics")
		return err
	}
	if reply.Done.Map["ret"] == "" {
		return nil
	}
	v, err := strconv.ParseFloat(reply.Done.Map["ret"], 32)
	if err != nil {
		log.WithFields(log.Fields{
			"ip_version": ipVersion,
			"device":     ctx.device.Name,
			"error":      err,
		}).Error("error parsing routes metrics")
		return err
	}

	ctx.ch <- prometheus.MustNewConstMetric(c.countDesc, prometheus.GaugeValue, v, ctx.device.Name, ctx.device.Address, ipVersion)
	return nil
}

func (c *routesCollector) colllectCountProtcol(ipVersion, topic, protocol string, ctx *collectorContext) error {
	reply, err := ctx.client.Run(fmt.Sprintf("/%s/route/print", topic), "?disabled=false", fmt.Sprintf("?%s", protocol), "=count-only=")
	if err != nil {
		log.WithFields(log.Fields{
			"ip_version": ipVersion,
			"protocol":   protocol,
			"device":     ctx.device.Name,
			"error":      err,
		}).Error("error fetching routes metrics")
		return err
	}
	if reply.Done.Map["ret"] == "" {
		return nil
	}
	v, err := strconv.ParseFloat(reply.Done.Map["ret"], 32)
	if err != nil {
		log.WithFields(log.Fields{
			"ip_version": ipVersion,
			"protocol":   protocol,
			"device":     ctx.device.Name,
			"error":      err,
		}).Error("error parsing routes metrics")
		return err
	}

	ctx.ch <- prometheus.MustNewConstMetric(c.countProtocolDesc, prometheus.GaugeValue, v, ctx.device.Name, ctx.device.Address, ipVersion, protocol)
	return nil
}
