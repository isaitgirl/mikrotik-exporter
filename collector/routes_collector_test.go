package collector

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRouteProtocol(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{name: "explicit protocol", values: map[string]string{"protocol": "static"}, want: "static"},
		{name: "connected alias", values: map[string]string{"protocol": "connected"}, want: "connect"},
		{name: "yes flag fallback", values: map[string]string{"bgp": "yes"}, want: "bgp"},
		{name: "true flag fallback", values: map[string]string{"bgp": "true"}, want: "bgp"},
		{name: "unknown", values: map[string]string{}, want: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, routeProtocol(test.values))
		})
	}
}

func TestRouteStatus(t *testing.T) {
	tests := []struct {
		name   string
		values map[string]string
		want   string
	}{
		{name: "disabled wins", values: map[string]string{"disabled": "true", "active": "true"}, want: "disabled"},
		{name: "active", values: map[string]string{"active": "true"}, want: "active"},
		{name: "inactive flag", values: map[string]string{"inactive": "yes"}, want: "inactive"},
		{name: "active false", values: map[string]string{"active": "false"}, want: "inactive"},
		{name: "unknown", values: map[string]string{}, want: "unknown"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, routeStatus(test.values))
		})
	}
}

func TestRouteFlagLabels(t *testing.T) {
	values := map[string]string{
		"active":   "true",
		"static":   "yes",
		"connect":  "false",
		"disabled": "no",
	}

	assert.Equal(t, "true", routeFlagLabel(values, "active"))
	assert.Equal(t, "true", routeFlagLabel(values, "static"))
	assert.Equal(t, "false", routeFlagLabel(values, "connected", "connect"))
	assert.Equal(t, "true", routeEnabledLabel(values))
	assert.Equal(t, "false", routeEnabledLabel(map[string]string{"disabled": "true"}))
}

func TestGatewayStatusLabel(t *testing.T) {
	assert.Equal(t, "true", gatewayReachableLabel(map[string]string{
		"gateway-status": "187.45.65.29 reachable via ether8",
	}))
	assert.Equal(t, "false", gatewayReachableLabel(map[string]string{
		"gateway-status": "10.99.0.1 inactive",
	}))
	assert.Equal(t, "false", gatewayReachableLabel(map[string]string{}))
}
