# 10 — Sistemas de jogo

Cada sistema abaixo corresponde a uma classe RTTI do `GameServer.exe`, ao(s) opcode(s) de cliente em [02 §7](02-protocolo-de-rede.md#7-tabela-de-opcodes-cliente--gameserver-observado), às operações do DataServer em [05](05-persistencia-data-server.md) e às chaves de configuração em `DATA/GameServerInfo - *.dat`. O comportamento interno de cada handler é trabalho das fases 3–5 do roadmap; aqui fica o escopo, o contrato e o que já foi observado.

## 1. Inventário de módulos do GameServer (observado, RTTI)

Agrupados por módulo Go sugerido:

| Módulo Go | Classes originais |
|---|---|
| `game/world` | `CMap`, `CMapManager`, `CMapServerManager`, `CMapRateInfo`, `CViewport`, `CObjectManager`, `CPath`, `CGate`, `CMove`, `CMoveSummon`, `CQueueTimer`, `CScheduleManager`, `CNotice`, `CMessage`, `CFilter`, `CRandomManager`, `CMemoryAllocator` |
| `game/object` | `CMonster`, `CMonsterManager`, `CMonsterMove`, `CMonsterSchedule`, `CMonsterSetBase`, `CMonsterSkillManager`, `CMonsterSkillUnit(Info)`, `CMonsterSkillElement(Info)`, `CNpcTalk`, `CMossMerchant`, `CMercenary`, `CGuardianStatue`, `CLifeStone`, `CCannonTower` |
| `game/combat`, `game/skill` | `CAttack`, `CSkill`, `CSkillManager`, `CSkillDamage`, `CSkillEnabler`, `CSkillHitBox`, `CSkillUseArea`, `CDamageTable`, `CPetDamageTable`, `CEffect`, `CEffectManager`, `CDarkSpirit`, `CFruit`, `CCustomComboSkill` |
| `game/character` | `CDefaultClassInfo`, `CDefaultClassFreebies`, `CExperienceTable`, `CResetTable`, `CResetLimit`, `CHelper`, `CHelperOffline`, `CAUTOHP` |
| `game/item` | ver [09 §4](09-itens-e-inventario.md#4-opções-e-taxas-observado) + `CInventoryEquipment`, `CItemBag`, `CItemBagManager`, `CMapItem` |
| `game/trade`, `pshop`, `shop`, `warehouse`, `chaosmix` | `CTrade`, `CPersonalShop`, `CShop`, `CShopManager`, `CWarehouse`, `CChaosBox`, `CJewelMix` |
| `game/party`, `guild`, `friend`, `duel` | `CParty`, `CGuild`, `CGuildManager`, `CUnionManager`, `CFriend`, `CDuel`, `CBattleSoccer`, `CGensSystem` |
| `game/quest` | `CQuest`, `CQuestObjective`, `CQuestReward`, `CQuestWorld`, `CQuestWorldObjective` |
| `game/event` | `CBloodCastle`, `CChaosCastle`, `CDevilSquare`, `CIllusionTemple`, `CKalima`, `CCastleSiege`, `CCastleSiegeCrown`, `CCastleSiegeCrownSwitch`, `CCastleSiegeSync`, `CCastleSiegeWeapon`, `CCastleDeep`, `CGoldenArcher`, `CInvasionManager`, `CDropEvent`, `CBonusManager`, `CEventSpawnStage`, `CEventEntryLevel`, `CEventHideAndSeek` |
| `game/custom` | `CCustomArena`, `CCustomAttack`, `CCustomBuyVip`, `CCustomEventDrop`, `CCustomEvo`, `CCustomGate`, `CCustomGift`, `CCustomHealthBar`, `CCustomJewel`, `CCustomJewelBank`, `CCustomJewelPack`, `CCustomLukeHelper`, `CCustomMarriage`, `CCustomMonster`, `CCustomMonsterKillCount`, `CCustomMove`, `CCustomNpcCommand`, `CCustomPet`, `CCustomPick`, `CCustomQuiz`, `CCustomRespawn`, `CCustomStore`, `CCustomWindowTime`, `CCustomWing`, `CCustomXShop`, `CustomDeathMessage`, `CLuckyWheel`, `cPackage` |
| `game/command` | `CCommandManager`, `CCommandRequirement`, `CGameMaster` |
| `game/security` | `CHackPacketCheck`, `CHackSkillSpeedCheck`, `CHackMoveSpeedCheck`, `CAntiFlood`, `CBlackList`, `CConnectMember`, `CSerialCheck`, `CProtect`, `CPacketManager` |
| `game/script` | `CScript`, `CScriptLoader`, `CLuaQueue` |
| infra | `CServerInfo`, `CServerDisplayer`, `CSocketManager`, `CSocketManagerUdp`, `CConnection`, `CQueryManager`, `CLog`, `CReconnect`, `CCriticalSection`, `CQueue` |

Classes de sistemas de versões posteriores (`CMasterSkillTree`, `CSocketItem*`, `C380Item*`, `CGensSystem`, `CIllusionTemple`, `CQuestWorld`) existem no binário; se estão ativas nesta configuração é incógnita (U-SYS-01).

## 2. Social

- **Party** (`40–43`): convite bloqueado em eventos; config `PartyMoneyDistribute`, `PartyDisableKillBetweenMembers`, `PartyGeneralExperienceN`, `PartySpecialExperienceN`, `PartyMaxGapLevel`.
- **Guild** (`50–66`, `E1–EB`): criação (`GuildCreateSwitch`, `GuildCreateMinLevel`, `GuildCreateMinReset`, validação de nome com `CheckNameSyntax`), membros (`GuildInsertMaxUserN`), alianças (`GuildAllianceMinUser`, `GuildAllianceMaxCount`), rivalidade/hostilidade, guerra de guild e Battle Soccer (`/guildwar`, `/guildsoccer`, `GuildWar*`). Persistência: DS `A0`.
- **Amigos e mail** (`C0–CB`): DS `B0`; SPs `WZ_FriendAdd/Del`, `WZ_WriteMail`.
- **Trade** (`36–3D`): `TradeSwitch`, `TradeItemBlock*` (bloqueio de excelentes/sets/vendidos). Bloqueado em evento (custom).
- **Loja pessoal** (`3F`): `PersonalShopSwitch`; log detalhado de venda com preços em joias (`JoB, JoS, JoL, JoC, JCr, JoG, JoH` — custom: preço em joias). `CustomStore*` e loja offline (`/offstore`, `CustomStoreOffline*`).
- **Duelo** (`AA–AC`): `DuelSwitch`, `DuelMaxScore`, `DuelMaxTime`, `DuelDamageRate`; ranking DS `3C`.
- **Casamento** (custom): `/marry`, `/accept`, `/divorce`, `/trackmarry`; `CustomMarriage*`; DS `0F/02`.

## 3. Progressão

- **Level up / pontos**: `MaxLevel` (`ServerMaxLevel = 400`), `<Classe>LevelUpPoint`, `MaxStatPoint`, `PlusStatPoint`, `/a /f /v /e /c` (adicionar pontos), `/readd`.
- **Reset / Master Reset** (custom): `/reset`, `/mreset`; `CommandReset*`, `CommandMasterReset*`, `Util/ResetTable.txt`, `Util/ResetLimit.txt`; DS `0F/00`, `0F/01`, `39`, `3A`.
- **Frutas**: `FruitAddPoint*`, `FruitSubPoint*`.
- **Quests** (`A0`, `A2`): `Data/Quest/Quest.txt`, `QuestObjective.txt`, `QuestReward.txt`; DS `0C` (`QuestKillCount`). Mensagem custom "Quest de Marlon concluída! Combo do BK liberado.".
- **VIP / AccountLevel**: AL0–AL3 em quase todas as configs; `/buyvip`, `/setvip`; `auth.SetAccountLevel`.

## 4. Eventos (observado: arquivos e chaves)

| Evento | Arquivo | Chaves em `Event.dat` |
|---|---|---|
| Blood Castle (mapas 11–17) | `Event/BloodCastle.dat` | `BloodCastleEvent`, `MaxUser` 10, `MaxEntryCount_AL*`, `NPCWithoutEntrance`, `DuelEnable` |
| Devil Square (9, 32) | `Event/DevilSquare.dat` | `DevilSquareEvent`, `MaxUser` 15, … |
| Chaos Castle (18–23) | `Event/ChaosCastle.dat` | `ChaosCastleEvent`, `MinUser`, `BlowUserRate`, `MoneyRate`, … |
| Castle Siege (30, servidor 19) | `Event/CastleSiege.dat` | `CastleSiegeEvent` (0 aqui), ciclo/estado (`CycleStart*`, `StateStart*`), dano de armas, `RegisterMinMasterLevel` 150, `RegisterMinGuildMember` 10 |
| Castle Deep | `Event/CastleDeepEvent.dat` | `CastleDeepEvent`, `CastleDeepEventTime` |
| Crywolf (34) | `Event/Crywolf.dat`, `Custom/Crywolf.dat` | — |
| Kalima (24–29, 36) | `Event/Kalima.dat` | — |
| Kanturu | `Event/Kanturu.dat` | — (mapas de Kanturu não estão no MapManager) |
| Illusion Temple | `Event/IllusionTemple.dat` | — (ranking DS `40`) |
| Golden Archer | `Event/GoldenArcher.dat` | DS `94`, cliente `95–97` |
| Invasões | `Event/InvasionManager.dat` | `InvasionManagerSwitch` |
| Drop event | `Event/DropEvent.dat` | `DropEventSwitch` |
| Bônus | `Event/BonusManager.dat` | `BonusManagerSwitch` |
| Entrada por nível | `Event/EventEntryLevel.dat` | contadores DS (`WZ_*EventEntryInfo`) |
| Spawns por estágio | `Event/EventStageSpawn.dat` | — |
| Hide and Seek, Quiz | `EventItemBag/Special/HideAndSeek.txt`, `Custom/CustomQuiz.txt` | `EventHideAndSeek*`, `CustomQuizSwitch` |
| Eventos de GM custom | `Custom/Event/{Death,Duel,Pair,Raffle,Roleta,Sobre,Theft}Event.txt` | comandos `/escesc`, `/prainhapvp`, `/matamata`, `/dupla`, `/rodar`, `/patente` |

`WindowEventSwitch` + `CCustomWindowTime`: janela de horários de eventos no cliente (o patch do cliente tem `WindowEventTimeSwitch = 1`).

Formato dos `.dat` de evento: texto com seções numeradas (mesma convenção dos `.txt`) — parser a escrever na fase 5.

## 5. Castle Siege

Estado persistido em `MuCastle_*` (DS `80–89`). Componentes: coroa e interruptores (`CCastleSiegeCrown*`), armas de cerco (`B7`), guardiões, torres, portões, NPCs compráveis/reparáveis/atualizáveis (`B2 03/04/05/07`), impostos (`B2 09`, `TAX_RATE_CHAOS/STORE`, `TAX_HUNT_ZONE`), registro de guilds e marcas (`B2 10`, `REG_MARKS`), cronograma (`WZ_CS_ModifyCastleSchedule`). Roda no servidor de código 19 (`CastleSiege.exe`).

## 6. Customizações WizardTeam (observado)

| Sistema | Evidência | Notas |
|---|---|---|
| CustomXShop (cash shop) | `FA 05/07/08/0E/10/12`, DS `0F/20–23`, `Custom/CustomXShop.txt`, `CustomXShopRecharge.txt`, imagens `pixqr/*.bmp` | Recarga via PIX (QR em BMP baixado por HTTP — strings `Falha ao baixar ImagemURL`), preço editável por GM, débito atômico de `Credits` |
| Jewel Bank | `F3 F5/F6`, DS `F7` | depósito/saque de joias por conta |
| Custom Attack / Offline Attack | `F3 0A/0B`, `/attack`, `/offattack`, `Custom.dat CustomAttack*` | auto-attack com skill escolhida; poção/reparo automáticos; offline com tempo limite |
| Custom Pick | `/pick`, `/pickset`, `/pickclear`, DS `13`, `CustomPick*` | auto-coleta filtrada |
| Lucky Wheel / Roleta | `FB 16`, `Custom/CustomLuckyWheel.txt` | |
| Marriage, Gift, Daily Reward, Buy VIP, Evolution (`/evo`), Health bar, Death message, Monster kill count, Respawn, Guard, Gate, Move, NPC command, Quiz, Arena, Mob combo, Luke helper, Event drop, Summon scroll | `Data/Custom/*.txt` | |
| Rankings extras (Patente, TopDeath, Duel*, TopRoleta…) | DS `41`, `F3`, `DB/Query/*.sql` | coluna dinâmica — substituir por enum |
| eZen | `/ezen`, `eZentaxa`, `eZenReward` | moeda alternativa |
| Verificação de cliente / HWID | `F3 09` | `MaxConnectionPerHID`, `CustomerHardwareId` |

Decisão de produto para a reescrita: implementar primeiro o **núcleo 97D** (sem customs), mantendo os pontos de extensão; depois portar os customs priorizados pelo dono do servidor.

## 7. Comandos de chat (observado, `Data/CommandManager.txt`)

Colunas: `Index AL0 AL1 AL2 AL3 Syntax Delay GMLevel MinLevel MaxLevel MinReset MaxReset Money_AL0..AL3`. Requisitos extras em `CommandRequirement.txt` (`FUN_00440660`, depois das checagens da tabela e antes do comando: as linhas cujo AL, nível, reset e classe casam com o personagem exigem equipamento vazio, itens, efeito, estado de quest, PK, cargo de guild, mapa/área e dia/hora UTC; a primeira falha manda `FailMessage` e o comando não roda; `cmdrequirement.go`); GMs em `Util/GameMaster.txt` (conta, personagem, nível GM).

Comandos: `/move /guildwar /guildsoccer /request /post /f /a /v /e /c /pk /zen /evo /bau /reset /mreset /clearinv /readd /rename /dcfriend /open /classe /ezen /autopotion /lock /unlock /resp /store /offstore /attack /offattack /gift /marry /accept /divorce /trackmarry /buyvip /pack4155 /unpack /pick /pickset /pickclear /gmove /gg /trace /track /dc /firework /make /drop /skin /setzen /hide /edit /setpk /setvip /moveall /moveguild /mob /makeset /banacc /banchar /banchat /unbanacc /unbanchar /unbanchat /bcstart /dsstart /ccstart /setbuff /escesc /prainhapvp /matamata /dupla /patente /rodar`.

Mensagens: `Data/Lang/{Eng,Por,Spn}/Message.txt` e `Notice.txt` (`ServerLang` em `Common.dat`).

## 8. Segurança e anti-cheat (observado)

- `HackPacketCheck.txt`: por head, limite de contagem/tempo, exigência de criptografia (C3/C4) e serial.
- `HackSkillSpeedCheck.txt` (`FUN_00488550`, `skillspeed.go`): regras por skill, classe, nível, reset e faixa de velocidade (mágica para o DW, de ataque para os outros). `MaxDelay` ms zera a contagem; acima de `MinCount` e de `MaxCount` usos a skill é recusada (o `C3 08 19`/`C3 09 1E` não sai, o passo da Nova é pulado) e `Action` 1 manda a mensagem 0x2CF, 2 fecha o jogo (contagem de 6 s), 3 manda a 0x2CF como aviso tipo 2 e marca `obj+0x2094` = agora + 10 s.
- `CheckSpeedHack*`, `CheckLatencyHack*`, `CheckAutoPotionHack*`, `CheckAutoComboHack*`, `CheckMoveHack*`, `AntiFlood*`, `TeleportAttackCheck`.
- `Data/BlackList.txt` (IPs recusados no `WSAAccept` do GameServer, HWIDs que fecham o cliente no `F3 09`; `blacklist.go`; não confundir com o `BlackList.txt` do ConnectServer), `ConnectMember.txt` + `ConnectMemberCheck` (lista de contas permitidas), `PersonalCodeCheck` (exige `sno__numb` para apagar personagem etc.).
- Proteção do cliente: `CProtect` (lado servidor) e `Data/Common/Protect/{Encrypt,Decrypt}.dat` (uso **não identificado**, U-SYS-02).
