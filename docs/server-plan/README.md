# Plano de reconstrução — Servidor (backend)

Especificação técnica para reescrever o servidor MU Online "99z / 97D" (um **emulador customizado**) como **um único serviço Go** com **PostgreSQL**, mantendo compatibilidade de protocolo com o cliente original.

Toda afirmação aqui foi extraída dos binários e arquivos originais com o REA (Ghidra 12.1.4 via MCP). Cada seção indica o nível de confiança:

- **Observado** — lido diretamente do código decompilado, de bytes do binário ou de arquivos de dados.
- **Inferido** — dedução a partir de evidência observada (ex.: nome de classe RTTI + strings de log). Precisa de validação em runtime.
- **Desconhecido** — listado em [12-roadmap-e-incognitas.md](12-roadmap-e-incognitas.md).

## Artefatos analisados

| Processo original | Binário | SHA-256 (prefixo) | Porta(s) |
|---|---|---|---|
| ConnectServer | `MuServer/1.ConnectServer/ConnectServer.exe` (x86, 397 KB) | `1b415695…` | TCP 44405 (clientes), UDP 55557 (heartbeats) |
| DataServer | `MuServer/2.DataServer/DataServer.exe` (x86, 518 KB) | `bcdc46f3…` | TCP 55960 (GameServers) |
| JoinServer | `MuServer/3.JoinServer/JoinServer.exe` (x86, 414 KB) | `7698c4a6…` | TCP 55970 (GameServers) |
| GameServer | `MuServer/4.GameServer/GameServer.exe` (x86, 4 MB, 6157 funções) | `f95eea53…` | TCP 55900 (clientes) |
| GameServerCS | `MuServer/5.GameServerCS/CastleSiege.exe` (x86, 3,9 MB) | não analisado em profundidade | TCP 55919 (código 19) |

Também foram usados: o dump do banco `MuServer/DB/MuOnline.sql` (UTF-16, SQL Server), os scripts em `MuServer/DB/Query/`, os `.ini`/`.dat` de configuração e toda a árvore `MuServer/Data/`.

> O binário `CastleSiege.exe` é uma build do mesmo GameServer configurada para o mapa de Castle Siege (mesmo conjunto de `DATA/GameServerInfo - *.dat`). A reescrita trata Castle Siege como um módulo do serviço único. A equivalência de código entre os dois executáveis ainda não foi comprovada (ver incógnitas).

## Visão geral da arquitetura original

```
Cliente ──TCP 44405──► ConnectServer ◄──UDP 55557── GameServer (heartbeat: código, usuários, max)
   │                         ▲         ◄──UDP 55557── JoinServer (heartbeat: tamanho da fila)
   │                         │
   └──TCP 55900──► GameServer ──TCP 55970──► JoinServer ──ODBC──► SQL Server (MEMB_INFO, MEMB_STAT…)
                        │
                        └─────TCP 55960──► DataServer ──ODBC──► SQL Server (Character, warehouse, Guild…)
```

Cada processo usa o mesmo framework de rede (IOCP + fila com semáforo + thread de processamento única) e o mesmo enquadramento de pacotes `C1/C2` (ver [02-protocolo-de-rede.md](02-protocolo-de-rede.md)).

## Arquitetura alvo (resumo)

Um binário Go (`apps/server`) com módulos internos:

| Módulo Go | Substitui | Responsabilidade |
|---|---|---|
| `connect` | ConnectServer | Lista de servidores e endereço do GameServer para o cliente |
| `auth` | JoinServer | Login, sessão única por conta, nível VIP, troca de servidor |
| `persistence` | DataServer | Repositórios PostgreSQL (personagem, baú, guild, castle…) |
| `game` | GameServer (+GameServerCS) | Mundo, objetos, combate, itens, eventos, sistemas |
| `protocol` | (comum) | Enquadramento, criptografia, codecs de pacotes |

Os protocolos internos GS↔JS e GS↔DS deixam de ser TCP e viram chamadas de interface Go; os opcodes originais são documentados apenas como contrato funcional. Detalhes em [01-arquitetura-alvo.md](01-arquitetura-alvo.md).

## Índice

| Documento | Conteúdo |
|---|---|
| [01-arquitetura-alvo.md](01-arquitetura-alvo.md) | Desenho do serviço Go único, concorrência, módulos, configuração |
| [02-protocolo-de-rede.md](02-protocolo-de-rede.md) | Enquadramento, criptografia (XOR32, SimpleModulus, "Bux"), tabela completa de opcodes cliente↔GameServer, layouts de pacotes |
| [03-connect-server.md](03-connect-server.md) | Lista de servidores, heartbeats UDP, limites anti-flood |
| [04-auth-join-server.md](04-auth-join-server.md) | Fluxo de login, códigos de resultado, MD5, sessão duplicada, troca de mapa/servidor |
| [05-persistencia-data-server.md](05-persistencia-data-server.md) | Todas as operações do DataServer (opcodes → SQL) |
| [06-schema-postgresql.md](06-schema-postgresql.md) | Schema original (MSSQL) e proposta de schema PostgreSQL, formatos de blobs |
| [07-mundo-e-objetos.md](07-mundo-e-objetos.md) | Modelo de objetos, índices, mapas/terreno, viewport, movimento, monstros, gates |
| [08-combate-e-formulas.md](08-combate-e-formulas.md) | Fórmulas de atributos, acerto, defesa, velocidade, taxas de dano PvP/PvM |
| [09-itens-e-inventario.md](09-itens-e-inventario.md) | Índice de item, formato binário (16 e 7 bytes), opções, drops, lojas, mix |
| [10-sistemas-de-jogo.md](10-sistemas-de-jogo.md) | Party, guild, trade, loja pessoal, duelo, amigos/mail, quests, eventos, Castle Siege, comandos, customs |
| [11-arquivos-de-configuracao.md](11-arquivos-de-configuracao.md) | Inventário e formato dos arquivos de dados e `.dat`/`.ini` |
| [12-roadmap-e-incognitas.md](12-roadmap-e-incognitas.md) | Fases de implementação, estratégia de verificação, incógnitas abertas |
| [13-progresso.md](13-progresso.md) | **Progresso da reconstrução**: o que já foi implementado, descobertas novas e próximo passo |
| [18-teste-de-carga.md](18-teste-de-carga.md) | Teste de carga (`muserver -seed-load/-metrics-addr`, `cmd/muload`), resultados e dimensionamento |

## Convenções

- Bytes multi-byte em pacotes: quando não indicado, **little-endian** (padrão x86). Campos marcados `BE` são big-endian (ex.: tamanho de `C2`, índices de objeto em vários pacotes).
- Endereços como `0x4d6690` referem-se ao binário indicado no contexto, com base de imagem `0x400000`.
- "AL0..AL3" = nível de conta (0 = Free, 1–3 = VIP), usado em quase todas as configurações.
