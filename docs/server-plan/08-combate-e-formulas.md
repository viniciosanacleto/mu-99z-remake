# 08 — Combate e fórmulas

## 1. Fonte de verdade

Existem duas implementações das fórmulas de personagem no original:

1. **Nativa** (`GameServer.exe`, função de cálculo `0x4c9790`), parametrizada pelas constantes de `DATA/GameServerInfo - Character.dat`, lidas em `CServerInfo` (`GetPrivateProfileIntA("GameServerInfo", "<chave>", 0, ".\Data\GameServerInfo - Character.dat")`).
2. **Lua** (`Data/Script/Character/CalcCharacter.lua`), usada apenas se `CalcLuaScriptSwitch = 1`.

Nesta instalação `CalcLuaScriptSwitch = 0` ⇒ **a nativa é a fonte de verdade**. As constantes do `.dat` **diferem** dos números escritos no Lua (ex.: DW dano físico mínimo: `.dat` = 6, Lua = 8; Elf defesa: `.dat` = 8, Lua = 10). A reescrita deve implementar a forma nativa com as constantes do `.dat`.

Todas as divisões são inteiras (C `int`).

## 2. Classes (observado)

| Código no DB/pacote | Classe | Índice nas fórmulas |
|---|---|---|
| 0 (`0x00`) | Dark Wizard (DW) | 0 |
| 16 (`0x10`) | Dark Knight (DK) | 1 |
| 32 (`0x20`) | Fairy Elf (FE) | 2 |
| 48 (`0x30`) | Magic Gladiator (MG) | 3 |
| 64 (`0x40`) | Dark Lord (DL) | 4 |
| 80 / 96 (`0x50`/`0x60`) | aceitas na criação (F3 01) se liberadas por `ExtClass` | 5 / 6 — o código de cálculo tem 7 ramos, mas não há dados (`DefaultClassInfo`, `DefaultClassType`) para elas nesta instalação |

O byte de classe guarda a evolução nos bits baixos (ex. `Class` no DB = `classe*16 + evolução` — **inferido**; validar com personagens evoluídos, U-COMBAT-02).

## 3. Atributos base por classe (observado, `Data/Character/DefaultClassInfo.txt`)

| Classe | Str | Dex | Vit | Ene | Lead | MaxLife | MaxMana | LevelLife | LevelMana | VitalityToLife | EnergyToMana |
|---|---|---|---|---|---|---|---|---|---|---|---|
| DW | 18 | 18 | 15 | 30 | 0 | 60 | 60 | 1.0 | 2.0 | 2.0 | 2.0 |
| DK | 28 | 20 | 25 | 10 | 0 | 110 | 20 | 2.0 | 0.5 | 3.0 | 1.0 |
| FE | 22 | 25 | 20 | 15 | 0 | 80 | 30 | 1.0 | 1.5 | 2.0 | 1.5 |
| MG | 26 | 26 | 26 | 26 | 0 | 110 | 60 | 1.0 | 1.0 | 2.0 | 2.0 |
| DL | 26 | 20 | 20 | 15 | 25 | **40090** | 40 | 1.5 | 1.0 | 2.0 | 1.5 |

O `MaxLife = 40090` do DL parece erro de digitação no arquivo de dados desta instalação (o `DefaultClassType` do banco tem 90). A fórmula exata de vida/mana máxima por nível não foi extraída (U-COMBAT-03); a expectativa, pelos nomes das colunas, é `MaxLife + (Level-1)*LevelLife + (Vit - VitBase)*VitalityToLife`.

`Data/Character/DefaultClassFreebies.txt` define zen, pontos, itens e efeitos iniciais por classe. São dados uma vez, na primeira entrada do personagem (`IsNewChar` da resposta 04 do DataServer, `FUN_0046d0b0`): zen com teto de 2.000.000.000, pontos (`F3 E0` + save), itens no inventário e efeitos do `EffectList`. Detalhes em [16](16-cobertura-dados.md#usos-no-jogo); implementação em `freebies.go`.

## 4. Dano físico e mágico (observado em `0x4c9790`)

`Str`, `Dex`, `Ene`, `Lead` = valores totais (base + bônus). Constantes = `<Classe><Nome>ConstA/B` do `Character.dat`.

| Classe | Físico mín. (dir/esq) | Físico máx. (dir/esq) | Mágico mín. | Mágico máx. |
|---|---|---|---|---|
| DW | `Str / DWPhysiDamageMinConstA` (6) | `Str / DWPhysiDamageMaxConstA` (4) | `Ene / DWMagicDamageMinConstA` (9) | `Ene / DWMagicDamageMaxConstA` (4) |
| DK | `Str / 8` | `Str / 4` | `Ene / 9` | `Ene / 4` |
| FE sem arco | `(Str+Dex) / 7` | `(Str+Dex) / 4` | `Ene / 9` | `Ene / 4` |
| FE com arco | `Str / 14 + Dex / 7` | `Str / 8 + Dex / 4` | idem | idem |
| MG | `Str / 6 + Ene / 12` | `Str / 4 + Ene / 8` | `Ene / 9` | `Ene / 4` |
| DL | `Str / 7 + Ene / 14` | `Str / 5 + Ene / 10` | `Ene / 9` | `Ene / 4` |

(Valores entre parênteses / números = configuração atual.) Ajustes do Lua que provavelmente existem também na nativa, mas **não foram verificados** no código nativo (U-COMBAT-04): cajado (`GET_ITEM(5,x)`) na mão reduz dano físico pela metade; dois armamentos (DK/MG) ⇒ 55% do dano em cada mão.

## 5. Taxas de acerto, defesa e velocidade (observado)

```
AttackSuccessRate (PvM) = (Level + MasterLevel?) * ConstA + (Dex * ConstB) / ConstC + Str / ConstD   [+ Lead / ConstE para DL]
  DW/DK/FE/MG: A=5, B=3, C=2, D=4      DL: A=5, B=6, C=2, D=4, E=10
PhysiSpeed  = Dex / <Classe>PhysiSpeedConstA   (DW 20, DK 15, FE 50, MG 15, DL 10)
MagicSpeed  = Dex / <Classe>MagicSpeedConstA   (DW 10, DK 20, FE 50, MG 20, DL 10)
DefenseSuccessRate (PvM) = Dex / <Classe>DefenseSuccessRateConstA   (DW 3, DK 3, FE 4, MG 3, DL 4)
Defense  = Dex / <Classe>DefenseConstA   (DW 3, DK 3, FE 8, MG 3, DL 3)
```

O termo somado ao nível (`obj+0x5e4`) é **inferido** como nível master/extra; nesta versão provavelmente 0 (U-COMBAT-05).

Fórmulas PvP de acerto/defesa (no Lua: `Level*3 + Dex*k`, `Level*2 + Dex/k`) — a versão nativa e suas constantes não foram localizadas (U-COMBAT-06).

Bônus de set +10..+13 (Lua `CalcBonusDefense`): defesa +5/10/15/20%.

## 6. Multiplicadores de dano configuráveis (observado, `Character.dat`)

- `GeneralDamageRatePvP/PvM`, `ReflectDamageRatePvP/PvM` (100).
- `<Classe>DamageRatePvP` / `PvM` (DW 110, DK 95, FE 115, MG 110, DL 115).
- Matriz `<Atacante>DamageRateTo<Alvo>` (5×5) e `<Atacante>ReflectDamageRateTo<Alvo>`.
- `DuelDamageRate` 60, `CustomArenaDamageRate` 50, `ChaosCastleDamageRate` 50, `CastleSiegeDamageRate1/2/3` 40/0/5.
- `<Classe>DamageStuckRate`, `DamageStuckOnPetUniria/Dinorant/DarkHorse`.
- `DKDamageMultiplierConstA/MaxRate`, `DLDamageMultiplierConstA/MaxRate`.
- Dark Spirit (corvo do DL): `DarkSpirit*` (taxa de ataque, crítico, dano mín./máx. A/B/C, velocidade A–D, acerto A–C).
- Combo: `<Classe>ComboDamageConstA/B/C`.
- Skills: `EarthquakeDamageConstA/B/C`, `ElectricSparkDamageConstA/B`, `DLSkillDamageConstA/B`, `NovaDamageConstA/B/C`.
- Regeneração por classe: `<Classe>HPRecoveryRate` (0), `MPRecoveryRate` (4), `BPRecoveryRate` (3; DK 5).

## 7. Skills (observado)

`Data/Skill/SkillList.txt`: `Index Name Damage MP BP Range Radio Delay Type Effect ReqLevel ReqEnergy ReqLeadership ReqKillCount ReqGuildStatus DW DK FE MG DL` (ex.: `1 "Poison" 12 42 0 6 0 0 1 55 …`). `Type = *` em algumas linhas.

Demais: `SkillDamage.txt`, `SkillUseArea.txt`, `SkillEnabler.txt`, `ComboSkill.txt`, `EffectList.txt`, `SkillElect.hit` (hitbox), `Util/DamageTable.txt`, `Util/PetDamageTable.txt`. Velocidade de skill limitada por `Hack/HackSkillSpeedCheck.txt`.

## 8. Fluxo de ataque (a detalhar)

`CAttack` (`D7`), `CSkillManager` (`10`, `19`, `1E`). Distâncias observadas: corpo a corpo 3, Elf 6. A sequência completa (acerto vs. defesa, crítico/excelente, redução por defesa, dano mínimo, reflexo, SD/escudo, morte, experiência, drop) precisa ser decompilada em detalhe a partir de `CAttack::Attack` — ver roadmap, fase 4, e U-COMBAT-07.

Experiência: `Util/ExperienceTable.txt` (taxas por faixa de nível/reset/master reset e AL), `MapManager.ExperienceRate`, `CustomAttack*ExperienceRate`. A tabela de experiência por nível (`ExperienceTable.txt` vazio nesta instalação) é gerada por `FUN_0050ad00` (**resolve U-COMBAT-08**): `exp[L] = (L+9)·A·L²` para L = 1..1000 e, para L > 255, soma `(k+9)·B·k²` com `k = L-255` (A/B = `ExperienceMultiplierConstA/B`, aritmética de 32 bits); pets: `(L+10)·C·L³` para L > 2 (`PetExperienceMultiplierConstA`). `NextExp = exp[nível]` (`FUN_0050be30`); a experiência carregada nunca fica abaixo de `exp[nível-1]`.
