# Memorias do repositorio

Notas para futuras manutencoes e agentes que trabalharem neste projeto.

## Projeto

- O projeto e um exporter Prometheus em Go para dispositivos MikroTik.
- A comunicacao com RouterOS usa a API binaria via `gopkg.in/routeros.v2`.
- O nucleo dos collectors fica em `collector/`; cada categoria implementa `routerOSCollector`.
- As opcoes de collectors sao registradas em `collector/collector.go` e ativadas por flags CLI e por `config.Config.Features` em `main.go`.
- O modulo declara Go 1.25 em `go.mod`.

## Rotas

- O collector de rotas existente preserva as metricas agregadas `mikrotik_routes_total_count` e `mikrotik_routes_protocol_count`.
- A coleta agregada usa `/ip/route/print` para IPv4 e `/ipv6/route/print` para IPv6.
- A coleta detalhada e opt-in por `features.routes_detail: true` ou `-with-routes-detail`.
- O modo detalhado usa `/ip/route/print` e `/ipv6/route/print` com `=detail=` e sem `.proplist`; no RouterOS testado, os flags calculados nao aparecem quando `.proplist` e usado.
- Rotas nao-BGP podem gerar metricas individuais com destino, gateway, comment, distance, scope e target-scope, usando labels booleanos `active`, `static`, `connected`, `enabled` e `gateway_reachable`.
- Prefixos BGP nao devem ser usados como labels: o modo detalhado agrega BGP por status em `mikrotik_routes_protocol_status_count`.
- A distancia (`distance`) faz parte dos labels de todas as metricas detalhadas para distinguir rotas com o mesmo destino e gateway; a metrica `mikrotik_routes_distance` tambem mantem a distancia como valor.
- `comment` e tratado como o nome legivel da rota. As flags `active`, `static`, `connected` e `enabled` sao publicadas como strings `true`/`false`.
- `gateway_reachable` e normalizado a partir de `gateway-status` para `true` ou `false`; estados `inactive`, ausentes ou desconhecidos sao tratados como `false`.
- O estado agregado do BGP continua sendo normalizado a partir das propriedades retornadas pela API. Nao assumir que existe uma propriedade textual universal chamada `status`.
- A documentacao oficial do RouterOS 7 mostra `/routing/route/print` como menu mais amplo, mas a implementacao atual prioriza `/ip/route/print` conforme o requisito operacional.

## Cardinalidade e compatibilidade

- Nao habilitar coleta detalhada de BGP por prefixo. Roteadores com muitas rotas BGP podem gerar cardinalidade excessiva.
- A biblioteca RouterOS usada e antiga, mas o protocolo API e baseado em comandos e atributos. Confirmar em equipamento real os nomes das flags e propriedades retornadas pelo RouterOS 7.
- Rotas ECMP e campos ausentes ainda precisam ser validados em um RouterOS 7 real.
- Alteracoes devem preservar as metricas agregadas existentes, salvo decisao explicita de breaking change.

## VPN health

- `collector/vpn_health_collector.go` le o estado do scheduler RouterOS (address-list `<prefix>_health-state`) e objetos `<prefix>_<tunnel>_{bgp,out-active,data-*}`; tuneis vem de `vpn_health.tunnels` no YAML.
- Cada fonte (address-list, active-peers, bgp peer, advertisements, filter, policy) e independente: falha de uma nao esconde as outras.
- O reader `proto` da lib rejeita palavras de query (`?list=`); testes com query usam `newRawPair` em `vpn_health_collector_test.go`.
- Smoke test real em RouterOS 6.48 (RB1100 PowerPC) com usuario de monitoramento read-only passou sem erros.
- Nunca consultar `/ip/ipsec/installed-sa` (expoe chaves).

## RouterOS 7 (validado em CHR 7.24.5, 2026-10-09)

- A API v7 responde `!empty` para print vazio; a lib `routeros.v2` trata como erro e deixa o `!done` sem ler, deslocando todas as respostas seguintes. O mesmo ocorre com `!done` apos `!trap`. `collector/api_filter_conn.go` filtra ambos na conexao.
- Nao sondar menu inexistente para detectar versao (gera `!trap`): usar `routerOSMajor` (`/system/resource/print version`).
- v7: sessoes BGP se chamam `<connection>-<n>` (flag `established=true`); advertisements usam esse nome em `peer`; filtros sao `/routing/filter/rule` com `rule="if (...) { accept; }"`.

## Testes e validacao

- Rodar `gofmt` nos arquivos Go alterados.
- Rodar `go test ./...` e `go vet ./...` antes de concluir alteracoes.
- Rodar `git diff --check` para detectar whitespace.
- Os testes do collector usam o fake API baseado em `newPair` e `fakeServer` em `collector/cloud_collector_test.go`.
- Ainda falta um smoke test conectado a um RouterOS 7 real para confirmar o contrato final dos campos de rota.

## Higiene

- Nao colocar senhas, tokens ou configuracoes reais de dispositivos nestes documentos.
- Manter `CHANGELOG.md` atualizado quando uma funcionalidade, compatibilidade ou comportamento observavel for alterado.
