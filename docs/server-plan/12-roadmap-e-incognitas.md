# 12 — Roadmap de implementação e incógnitas

## 1. Fases

Cada fase termina com o cliente original funcionando até o ponto descrito, contra o servidor Go.

| Fase | Entrega | Critério de pronto |
|---|---|---|
| **0 — Ferramentas** | Proxy TCP de captura (cliente ↔ servidor original) que grava sessões em claro e cifradas; parser de config `Data/` com testes carregando 100% dos arquivos; implementações de XOR32, SimpleModulus e Bux com vetores de teste das capturas | Decifrar e re-cifrar sessões capturadas byte a byte |
| **1 — Login** | `protocol`, `connect`, `auth`, `persistence` (contas, personagens), migrations PostgreSQL, ferramenta de migração MSSQL | Cliente vê a lista de servidores, faz login, cria/apaga/lista personagens e entra no mundo (Lorencia vazia) |
| **2 — Mundo** | Mapas, atributos, viewport, movimento, ações, chat, whisper, gates/move, NPCs fixos, spawn de monstros sem IA | Dois clientes se veem andando e conversando; teleportes e `/move` funcionam |
| **3 — Combate e itens** | IA de monstros, ataque físico e skills, experiência e level up, drops, inventário, uso de itens, lojas, reparo, baú, chaos machine básica | Matar monstros, subir de nível, comprar/vender, guardar no baú, persistir tudo |
| **4 — Social** | Party, guild (+ alianças, guerra), trade, loja pessoal, amigos/mail, duelo, PK | Paridade com fluxos capturados do original |
| **5 — Eventos** | Blood Castle, Devil Square, Chaos Castle, Kalima, Golden Archer, invasões, quests; Castle Siege + troca de servidor | Agenda de eventos completa rodando |
| **6 — Customs WizardTeam** | XShop/créditos, Jewel Bank, attack/offline, pick, reset/master reset, rankings, marriage, eventos de GM | Lista priorizada pelo dono do servidor |

Paralelamente: observabilidade, testes de carga (1000 conexões por instância, como o original), hardening (queries parametrizadas, hash de senha, rate limiting).

## 2. Estratégia de verificação

1. **Capturas de referência**: rodar o servidor original (Windows/VM) e gravar sessões reais por funcionalidade. Cada pacote documentado vira teste golden.
2. **Teste diferencial**: o mesmo roteiro de cliente (bot) executado contra o original e contra o Go; comparar pacotes e estado no banco.
3. **REA em runtime** quando estática não basta (ex. fórmulas de vida/mana, IA): Frida/x64dbg no Windows para observar valores em `0x4c9790`, `CAttack`, `CMonster`.
4. Toda lacuna resolvida atualiza este documento e o documento do tópico.

## 3. Incógnitas abertas

| ID | Pergunta | Evidência atual | Como resolver |
|---|---|---|---|
| U-GSCS-01 | `CastleSiege.exe` é o mesmo código do GameServer com outra configuração? | Mesmos `DATA/*.dat`; binário não decompilado | Comparar funções (REA `compare_functions`/diff de strings) |
| ~~U-PROTO-05~~ | Resolvida: bytes 4–7 = `obj+0x274` u32 LE (ver doc 02) | `0x515510` | — |
| U-PROTO-06 | Heads "a confirmar" em [02 §7](02-protocolo-de-rede.md#7-tabela-de-opcodes-cliente--gameserver-observado) (04, 06, 19, 1B, 91, A7, A9, B0, B1, F2 05, F3 06/12/30/F7, FB 02) | Handlers localizados | Decompilar handlers + capturas |
| U-PROTO-07 | Layout campo a campo dos pacotes do catálogo S→C | Só headers coletados | Fase 2–5 |
| U-PROTO-09 | `F3 03`: o builder `0x473a10` foi lido como 116 bytes, mas o `Main.dll` lê 176 (layout no doc 02). Falta reler o builder (há um hook custom que estende o pacote?), saber onde o GS original manda o `F3 E0` na entrada, identificar os 30 bytes + 5 u32 custom (os mesmos do `F3 E0`) e onde o cliente recebe a Patente | `Main.dll` `0x1001d640`, `0x1001d820`; observado com o cliente em 2026-10-24 | Reabrir `0x473a10` no REA; captura com o servidor original |
| U-CS-01 | Timeout de heartbeat UDP no ConnectServer | — | Ler timer do CS |
| U-AUTH-01 | Como a conta entra no MD5 da senha (`MD5Encryption=1`) | `FUN_00406710(senha, conta…)` | Decompilar `FUN_00406710` |
| U-AUTH-02 | Offsets da resposta `C1 3C 01` JS→GS | struct local em `FUN_00404a50` | Ler struct |
| U-DS-01 | Onde a guild é criada (SP `WZ_GuildCreate` não aparece no DS) | — | Procurar no GS/DS por `G_Name` insert |
| U-DB-01 | Definição de `MEMB_INFO.Credits` e `Gold` | Usadas pelo DS, ausentes nos scripts | Ler o `.bak` ou o banco em produção |
| U-DB-02 | Schema de `QuestKillCount` | Colunas pelas queries do DS | idem |
| U-DB-03 | Schema de `RankingIllusionTemple` | idem | idem |
| U-DB-04 | Significado dos 3 bytes por skill em `MagicList` | Origem `+8`, `+4`, `+9` do registro | Ler `CSkill` |
| U-DB-05 | Formato de `ReconnectData.Data` (400 bytes) | `CReconnect` | Decompilar |
| ~~U-ITEM-02~~ | Resolvida (ver doc 09 §8) | — | — |
| U-ITEM-04 | Nomes das 16 colunas de `ItemDrop.txt` | Arquivo sem cabeçalho | Parser nativo (`CItemDrop`) |
| U-ITEM-05 | Receitas e taxas da Chaos Machine | `CChaosBox` | Decompilar handler `88` |
| ~~U-WORLD-01~~ | Resolvida: `0..0x5013` monstros/NPCs, `0x5014..0x53FB` summons (ver doc 07) | — | — |
| U-WORLD-02 | IA de monstros (aggro, perseguição, regen, respawn) | Classes identificadas | Decompilar `CMonster*` |
| U-COMBAT-02 | Codificação de evolução de classe no byte `Class` | — | Personagem evoluído + código |
| ~~U-COMBAT-03~~ | Resolvida: `MaxLife + LevelLife·(Level-1) + (Vit+AddVit-VitBase)·VitalityToLife` (mana igual); BP em `0x4c9410` (ver doc 08b) | — | — |
| ~~U-COMBAT-04~~ | Resolvida: cajado soma metade do dano; duas armas (DK/MG/DL, índices < 0x800) ×55% em cada mão (ver doc 08b) | — | — |
| ~~U-COMBAT-05~~ | Resolvida: `obj+0x5e4` = master level, só definido por Lua (0 aqui) | — | — |
| U-COMBAT-06 | Fórmulas PvP nativas | Lua tem versão | idem |
| U-COMBAT-07 | Pipeline completo de dano (crítico, excelente, defesa, reflexo, SD) | — | `CAttack` |
| U-SYS-01 | Sistemas de versões posteriores (Master skill, Socket, 380, Gens, Illusion Temple) estão ativos? | Classes no binário | Checar configs e handlers |
| U-SYS-02 | Uso de `Data/Common/Protect/{Encrypt,Decrypt}.dat` | Arquivos presentes | `CProtect` |
| U-PROTO-08 | O `Main.dll` deriva a cifra de fluxo (02 §2.5) de `CustomerName` + `ClientSerial` do mesmo jeito que o GS? | Lado servidor observado; cliente não decompilado | Decompilar `Main.dll` / captura |
| U-CFG-01 | Diferenças entre `4.GameServer/DATA` e `5.GameServerCS/DATA` | — | `diff` dos arquivos |

### Resolvidas

| ID | Resolução |
|---|---|
| U-PROTO-02 | `F1 00` montado em `0x4db050`: `C1 0E F1 00 01 idx(BE) versão[5] ServerCode(u16 LE)`. (2026-10-22) |
| U-PROTO-03 | Bloco final de `F3 03`: ResetCount, 12 u32 (LUP, vida, …, liderança), nível u16 em 108, Patente u32 em 112. (2026-10-22) **Substituída por U-PROTO-09** (o cliente lê outro layout). |
| U-PROTO-04 | Bytes 22–23 e 41–43 da entrada `C2 12` são padding de alinhamento; ViewSkillState em 24, nome em 28. (2026-10-22) |
| U-AUTH-03 | Tradução em `0x4a18d0` (códigos 1/2/5/6/8/10 + resultado do JS); ver 02 §4. (2026-10-22) |
| U-ITEM-03 | CharSet reconstruído (`0x4c6ba0`/`0x472d00`); ver 09 §2.4. (2026-10-22) |
| U-COMBAT-08 | Tabela gerada por fórmula em `FUN_0050ad00`; ver 08. (2026-10-22) |
| U-PROTO-01 | Índice inicial do XOR32 = posição do head (2 em C1, 3 em C2); observado em `FUN_004ceb30` (2026-10-22). Ver [02 §2.1](02-protocolo-de-rede.md#21-xor32-nos-pacotes-c1c2-cliente--gameserver-observado). |

## 4. Fora de escopo (por enquanto)

- `MuMaker MuKing/` e `Tools/GetMainInfo` (ferramentas do administrador; o `GetMainInfo.exe` gera o `License.wzt` do cliente a partir do `MainInfo.ini` — relevante para o plano do cliente, não do servidor).
- Backups `.bak`/`.rar` do banco.
