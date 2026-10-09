# Changelog

Todas as mudancas relevantes deste projeto devem ser registradas neste arquivo.

## [Unreleased]

### Fixed

- RouterOS 7: `bgp` e `vpn_health` escolhem os menus pela versao (`/system/resource`): v7 usa `/routing/bgp/connection|session` e `/routing/filter/rule`. No v7, `bgp` publica apenas `mikrotik_bgp_up` e `mikrotik_bgp_prefix_count` (sem contadores de updates/withdrawn).
- RouterOS 7.18+: respostas `!empty` e o `!done` apos `!trap` nao dessincronizam mais o client sync (`apiFilterConn`).

### Added

- Adicionada a feature `vpn_health` (flag `-with-vpn-health`) e a secao `vpn_health` no YAML para monitorar tuneis active/standby gerenciados por scheduler RouterOS:
  - `mikrotik_vpn_monitor_up`, `mikrotik_vpn_mode`
  - `mikrotik_vpn_tunnel_active`, `mikrotik_vpn_tunnel_healthy`, `mikrotik_vpn_tunnel_consecutive_failures`, `mikrotik_vpn_tunnel_consecutive_successes`
  - `mikrotik_vpn_tunnel_ike_up`, `mikrotik_vpn_tunnel_ike_uptime_seconds`, `mikrotik_vpn_tunnel_bgp_up`, `mikrotik_vpn_tunnel_advertised_prefixes`
  - `mikrotik_vpn_tunnel_export_accept`, `mikrotik_vpn_tunnel_data_policies_enabled`, `mikrotik_vpn_tunnel_data_policies_established`
- Adicionada a feature `routes_detail` na configuracao YAML.
- Adicionada a flag CLI `-with-routes-detail`.
- Adicionadas metricas detalhadas para rotas nao-BGP:
  - `mikrotik_routes_info`
  - `mikrotik_routes_distance`
  - `mikrotik_routes_scope`
  - `mikrotik_routes_target_scope`
- Adicionada a metrica agregada `mikrotik_routes_protocol_status_count` para acompanhar estados BGP sem expor prefixos como labels.
- Adicionados testes unitarios para normalizacao de protocolo e estado de rotas.
- Adicionada documentacao da coleta detalhada e do controle de cardinalidade no README.
- Adicionados os labels booleanos `active`, `static`, `connected` e `enabled` na metrica `mikrotik_routes_info`.
- Adicionado o label `gateway_reachable`, normalizado como `true` ou `false`.
- Adicionado o label `distance` em `mikrotik_routes_info` para distinguir rotas com o mesmo destino e gateway.

### Changed

- Metricas `mikrotik_ipsec_*` passam a ser `gauge` (antes `counter`); valores e labels inalterados.
- O modo detalhado consulta `/ip/route/print` e `/ipv6/route/print` com `=detail=` sem `.proplist`, pois os flags calculados nao aparecem na resposta RouterOS quando `.proplist` e usado.
- Rotas BGP sao agregadas por `ip_version`, `protocol` e `status` no modo detalhado.
- Rotas nao-BGP detalhadas expoem `dst_address`, `gateway`, `comment` e os labels booleanos `active`, `static`, `connected` e `enabled`, alem dos valores numericos de distancia e scopes.
- Os valores de flags aceitam respostas RouterOS em formatos `yes/no` e `true/false`.
- O label `gateway_reachable` e derivado do campo RouterOS `gateway-status`; `inactive` e tratado como `false`.
- Removido o label `route_id`; `distance` passou a identificar rotas com o mesmo destino e gateway em todas as metricas detalhadas, sem expor o identificador interno do RouterOS.
- O status agregado do BGP e normalizado como `active`, `inactive`, `disabled` ou `unknown`.
- Corrigida a coleta para reconhecer `static`, `connect`, `active` e `disabled` retornados pela API RouterOS.
- Corrigido o caminho da consulta IPv6, que usava `/ip/route/print` em vez de `/ipv6/route/print`.

### Compatibility

- A feature `routes` continua produzindo as metricas agregadas existentes.
- A coleta detalhada permanece opt-in para evitar aumento inesperado de cardinalidade Prometheus.
- A compatibilidade completa com RouterOS 7 ainda depende de smoke test em equipamento real para confirmar os nomes e a presenca das propriedades de flags retornadas pela API.

### Validation

- `go test ./...`
- `go vet ./...`
- `git diff --check`
