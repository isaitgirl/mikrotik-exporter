[![Docker Pulls](https://img.shields.io/docker/pulls/nshttpd/mikrotik-exporter.svg)](https://hub.docker.com/r/nshttpd/mikrotik-exporter/)

## prometheus-mikrotik

tl;dr - prometheus exporter for mikrotik devices

This is still a work in progress .. consider `master` at the moment as a preview
release.

#### Description

A Prometheus Exporter for Mikrotik devices. Can be configured to collect metrics
from a single device or multiple devices. Single device monitoring can be configured
all on the command line. Multiple devices require a configuration file. A user will
be required that has read-only access to the device configuration via the API.

Currently the exporter collects metrics for interfaces and system resources. Others
can be added as long as published via the API.

#### Mikrotik Config

Create a user on the device that has API and read-only access.

`/user group add name=prometheus policy=api,read,winbox`

If `lte` is enabled it requires also the `test` policy.

`/user group add name=prometheus policy=api,read,winbox,test`

Create the user to access the API via.

`/user add name=prometheus group=prometheus password=changeme`

#### Single Device

`./mikrotik-exporter -address 10.10.0.1 -device my_router -password changeme -user prometheus`

where `address` is the address of your router. `device` is the label name for the device
in the metrics output to prometheus. The `user` and `password` are the ones you
created for the exporter to use to access the API.

User and password flags can be set with the `MIKROTIK_USER` and `MIKROTIK_PASSWORD` environment variables, respectively.

```
MIKROTIK_USER=prometheus
MIKROTIK_PASSWORD=changeme
./mikrotik-exporter -address 10.10.0.1 -device my_router
```

#### Config File

`./mikrotik-exporter -config-file config.yml`

where `config-file` is the path to a config file in YAML format.

###### example config
```yaml
devices:
  - name: my_router
    address: 10.10.0.1
    user: prometheus
    password: changeme
  - name: my_second_router
    address: 10.10.0.2
    port: 8999
    user: prometheus2
    password: password_to_second_router
    wifiwave2: true
  - name: routers_srv_dns
    srv:
      record: _mikrotik._udp.example.com
    user: prometheus
    password: password_to_all_dns_routers
  - name: routers_srv_custom_dns
    srv:
      record: _mikrotik2._udp.example.com
      dns:
        address: 1.1.1.1
        port: 53
    user: prometheus
    password: password_to_all_dns_routers

features:
  bgp: true
  dhcp: true
  dhcpv6: true
  dhcpl: true
  routes: true
  routes_detail: true
  pools: true
  optics: true
  wlanif: true
  wlansta: true
```

`routes: true` keeps the aggregated route metrics. Use `routes_detail: true` to
collect individual non-BGP routes from `/ip/route/print` and
`/ipv6/route/print`. BGP prefixes are not exposed as labels; only counts by
status are exported to avoid high Prometheus cardinality.

Detailed route metrics include the destination, gateway, comment, distance
(also used as an identity label), scope, target scope, and boolean labels `active`, `static`, `connected`, and
`enabled`, plus `gateway_reachable` with values `true` or `false`.
The command-line equivalent is
`-with-routes-detail`.

###### VPN health (active/standby tunnels)

`vpn_health: true` reads the state written by a RouterOS health scheduler that
keeps one tunnel active among several IPsec/BGP tunnels. Object names are
derived from `prefix`: address-list `<prefix>_health-state` (summary
`<mode>-<A|U|N>-<tunnel>` at `summary_address`, per-tunnel state
`<healthy><failures><successes>` at `state_address`), BGP peer
`<prefix>_<tunnel>_bgp`, export filter comment `<prefix>_<tunnel>_out-active`
and IPsec data policies commented `<prefix>_<tunnel>_data-*`.

```yaml
features:
  vpn_health: true
vpn_health:
  prefix: aws_prdvpc
  summary_address: 198.18.0.10   # default
  devices: [my_router]           # optional; empty = all devices
  tunnels:
    - {name: a1, provider: algar, state_address: 198.18.0.11, remote_address: 18.220.90.8}
    - {name: u1, provider: unifique, state_address: 198.18.0.13, remote_address: 3.21.255.11}
```

Metrics (labels `name`, `address`, `tunnel`, `provider`):
`mikrotik_vpn_monitor_up`, `mikrotik_vpn_mode{mode,desired}`,
`mikrotik_vpn_tunnel_active`, `mikrotik_vpn_tunnel_healthy`,
`mikrotik_vpn_tunnel_consecutive_failures`, `mikrotik_vpn_tunnel_consecutive_successes`,
`mikrotik_vpn_tunnel_ike_up`, `mikrotik_vpn_tunnel_ike_uptime_seconds`,
`mikrotik_vpn_tunnel_bgp_up`, `mikrotik_vpn_tunnel_advertised_prefixes`,
`mikrotik_vpn_tunnel_export_accept`, `mikrotik_vpn_tunnel_data_policies_enabled`
and `mikrotik_vpn_tunnel_data_policies_established`. Only `print` commands are
used (no installed SAs). The command-line flag is `-with-vpn-health`, but the
tunnels must come from the config file.

If you add a devices with the `srv` parameter instead of `address` the exporter will perform a DNS query
to obtain the SRV record and discover the devices dynamically. Also, you can specify a DNS server to use
on the query.

Use the option `wifiwave2: true` for devices that have the `wifiwave2` package,
which replaces the `wireless` implementation, installed. This is necessary as `wifiwave2` has a slightly
different API and exposes a slightly smaller set of attributes (for example, no signal-to-noise, etc.)


###### example output

```
mikrotik_interface_tx_byte{address="10.10.0.1",interface="ether2",name="my_router"} 1.4189902583e+10
mikrotik_interface_tx_byte{address="10.10.0.1",interface="ether3",name="my_router"} 2.263768666e+09
mikrotik_interface_tx_byte{address="10.10.0.1",interface="ether4",name="my_router"} 1.6572299e+08
mikrotik_interface_tx_byte{address="10.10.0.1",interface="ether5",name="my_router"} 1.66711315e+08
mikrotik_interface_tx_byte{address="10.10.0.1",interface="ether6",name="my_router"} 1.0026481337e+10
mikrotik_interface_tx_byte{address="10.10.0.1",interface="ether7",name="my_router"} 3.18354425e+08
mikrotik_interface_tx_byte{address="10.10.0.1",interface="ether8",name="my_router"} 1.86405031e+08
```

 
