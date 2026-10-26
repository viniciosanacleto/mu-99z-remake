# 08b — CalcCharacter, nível, pontos, party e PK (achados do REA)

Complementa o [08](08-combate-e-formulas.md). Tudo observado no `GameServer.exe` (endereços citados); implementado em `apps/server/internal/game/{stats,levelup,party,pk}.go` e `internal/gamedata/charstats.go`.

## 1. CalcCharacter (`0x4c9790`)

Chamado por `FUN_004c6ba0` (CharSet) — logo, a cada entrada no mundo (antes do `F3 03`), troca de equipamento, troca, etc. — e por level up / ponto de status.

Ordem:

1. Guarda as razões atuais: `Life*100/(AddLife+MaxLife)` (float), mana idem, shield e BP (`(float)(BP*100)/(float)(AddBP+MaxBP)`).
2. Zera `AddLife/AddMana/AddBP/AddShield`, reseta os acumuladores de opções (`FUN_0050bc70`: taxas de exp/drop/zen = 100).
3. Laço de requisitos: `AddStr..AddLead = efeitos (+0x730..)`; opções de set ancient (não reproduzido, U-STATS-01); opção 105 (liderança `nível*valor+10`); um item deixa de valer (`CItem+0xb5 = 0`) se durabilidade 0, nível/força/agilidade/vitalidade/energia/comando insuficientes ou classe sem coluna no Item.txt (`FUN_00496d00`, `FUN_00494b50`). Repete até estabilizar.
4. Dano base por classe (doc 08 §4). **FE usa a fórmula de arco** quando a mão direita é arco (4,x ≠ 4,15) ou a esquerda é arco (≠ 4,7) — só pelo índice.
5. Multiplicadores: DK `min(Ene/DKDamageMultiplierConstA + 200, DKDamageMultiplierMaxRate)`, DL `min(Ene/DLDamageMultiplierConstA + 200, Max)` (energia nos dois).
6. Arma da mão direita (slot 0) soma ao dano direito, a da esquerda ao esquerdo; **cajados (categoria 5) somam metade**; item inválido ou durabilidade 0 soma 0.
7. Taxa de acerto: `B*Dex/C + Str/D + (Level + obj+0x5e4)*A` (+`Lead/E` no DL). **`obj+0x5e4` = nível master**, só alterado por Lua nesta build → 0 (resolve U-COMBAT-05).
8. Velocidade: `Dex/PhysiSpeedConstA` (+efeitos); arma direita e esquerda válidas ⇒ soma a média `(spdD+spdE)/2`; uma só ⇒ a dela. Luvas (slot 5), pet (8) e pingente (9) válidos somam sua AttackSpeed sem checar durabilidade.
9. Taxa de defesa = `Dex/DefenseSuccessRateConstA` + DSR dos slots 1..7; defesa = `Dex/DefenseConstA` + defesa dos slots 1..7 (válidos e com durabilidade).
10. Set completo: elmo, armadura, calça, luva, bota (slots 2..6; MG pula o elmo e conta +15) do mesmo tipo e válidos ⇒ DSR +10% e, com as 5 peças ≥ +10: +5% (+10), +10% (todas ≥ +11), +15% (≥ +12), +20% (≥ +13), +25% (≥ +14), +30% (todas +15) na defesa.
11. Dark Horse (13,4) no slot 8: `def = Dex/20 + def + 5 + nívelPet*2`. Dark Raven (13,5) do DL: `FUN_0046b0c0` com as constantes `DarkSpirit*`.
12. Resistências = máximo entre pingente e anéis (slots 9..11), sem checar validade.
13. Experiência mínima/próxima (`FUN_0050be30`).
14. Opções dos itens (`FUN_0049d4e0`, códigos 80..127 do ItemOption.txt; tabela em `stats.go applyOption`). As de % de vida/mana (90/91) usam o **MaxLife/MaxMana anterior** (ainda não recalculado).
15. Flechas/virotes: arco na esquerda (slot do Item.txt = 1) + flechas 4,15 de nível n na direita ⇒ `min += min*(2n+1)/100 + 1`, `max += novoMin*(2n+1)/100 + 1` (usa o mínimo já aumentado); besta na direita + virotes 4,7 na esquerda, idem na mão direita.
16. Duas armas (DK, MG, DL; ambos índices < 0x800, sem checar validade): os quatro danos ×55/100.
17. **Vida e mana (resolve U-COMBAT-03)** em float:
    `MaxLife = DefaultClassInfo.MaxLife + LevelLife*(Level-1) + (Vit+AddVit - VitBase)*VitalityToLife`, mana análoga com Energy/EnergyToMana. Os bases vêm de `DefaultClassInfo.txt` (o DL tem `MaxLife = 40090` nesse arquivo e é isso que o servidor usa).
18. **BP máximo (`FUN_004c9410`)**, truncado:
    DW `.2Str+.4Dex+.3Vit+.2Ene`; DK `.15Str+.2Dex+.3Vit+Ene`; FE `.3Str+.2Dex+.3Vit+.2Ene`; MG `.2Str+.25Dex+.3Vit+.15Ene`; DL `.3Str+.2Dex+.1Vit+.15Ene+.3Cmd`.
19. CustomAttribute.txt (`FUN_00448070`) para todo item equipado.
20. Reaplica as razões do passo 1 (com teto no máximo) e envia **`C1 CC F3 E1`** (`FUN_004dc150`) e **`C1 3C F3 E3`** (`FUN_004dcc20`), pacotes custom com os atributos (layouts em `stats.go`).

Os pontos de `CItem::Convert` (`0x48aed0`) usados aqui estão em `convertItem`: excelente `dano*25/NívelItem + 5` (o máximo usa o dano mínimo original), ancient `(Nível+30)/40 + 10 + ...`, `+3/nível` e `(l-9)(l-8)/2` acima de +9, defesa de asas por `WingOption.txt DefenseConstA` (ou CustomWing), escudo `+nível`, fator de durabilidade (1 / .8 / .7 / .6 / .5 / 0 abaixo de 50/40/30/20% da máxima) e o ItemOption.txt (chave = slot de opção 0..13, vale a última regra que casa).

## 2. Experiência e level up

- `FUN_004c6670` (LevelUp): soma em u32; abaixo da próxima ⇒ só soma (retorna 0). Senão, por nível: `+<Classe>LevelUpPoint_AL<n>`, `+PlusStatPoint` se `Level > PlusStatMinLevel` e quest[0] bits 4-5 = 2; consome no máximo `MaxLevelUp` níveis — o excedente do último é descartado e a experiência fica no piso do novo nível. Depois: CalcCharacter, vida/mana/BP/shield cheios, `C1 30 F3 05` e `C1 06 48 idx 10` ao viewport; salva se passaram 60 s do último save de level up. Em `MaxLevel` só envia a mensagem 0x10a.
- `C1 30 F3 05` (`FUN_004db550`): `[4]` nível u16, `[6]` pontos u16, `[8]` vida máx u16, `[10]` mana máx, `[12]` BP máx, `[14]` fruta+ , `[16]` limite, `[18]` fruta−, `[20]` limite, `[24]` pontos u32, `[28]` vida máx u32, `[32]` mana, `[36]` BP, `[40]` exp, `[44]` próxima.
- `C3 18 9C` (`FUN_004dab00`): `[3..4]` monstro (bit 7 de `[3]` = flag), `[6]` exp alta u16, `[8]` exp baixa u16, `[10..11]` dano BE (65000 ⇒ `FD E8`), `[16]` exp atual, `[20]` próxima. **O valor de exp é 0 quando houve level up.**
- Matança sem party (`FUN_004c5970` → `FUN_004c5a30` para cada atacante da lista de dano): `base = (ml+25)*ml/3`, reduzida para `(ml+10)*base/pl` se `ml+10 < pl`, `+(ml-64)*(ml/4)` se `ml ≥ 65`; `exp = base + base/4`; `exp*dano/MaxLife` × (taxa do jogador, 100) × `AddExperienceRate_AL` /100 × MapManager ExperienceRate /100 × bônus /100 × CustomMonster /100 × ExperienceTable /100 × CustomAttack /100.
- **Party (`FUN_004c5ea0`)**: membros no mesmo mapa a < 10 células do monstro; nível de quem está 200+ abaixo do maior conta +200; taxa `PartySpecialExperience<n>` se ≥ 3 classes distintas, senão `PartyGeneralExperience<n>`; `base` usa a média de nível e, acima de 64, `+200 - Level(matador)/5` com 2+ membros; cada membro recebe `(nível*exp*taxa*n)/somaNíveis/100 * AddExperienceRate * taxaJogador/100 * ...` (aritmética u32), e o monstro acumula `/n`.

## 3. Pontos de status — `F3 06`

Cliente `C1 05 F3 06 tipo` (0..4 = for/agi/vit/ene/cmd; o comando vale para qualquer classe). `FUN_004c68f0`: precisa de pontos e de `atributo+1 ≤ MaxStatPoint_AL<n>` (senão mensagem 0x10c). Resposta `C1 30 F3 06` (`FUN_004d98f0`): `[4]` tipo+0x10 (0 = falha), `[6]` vida máx (tipo 2) ou mana máx (tipo 3) u16, `[8]` BP máx u16, `[12..44]` u32: pontos, vida máx, mana máx, BP máx, for, agi, vit, ene, cmd.

## 4. Party (`CParty`)

- `40` (C3 no HackPacketCheck): `[3..4]` alvo. Exige mapa com PartyEnable, ninguém com pedido pendente, diferença de nível ≤ `PartyMaxGapLevel` (senão resultado 5), quem pede ser líder (ou sem party), alvo aceitando pedidos (`obj+0x434` bit 0, padrão 3; senão 1) e sem party (senão 4). Sucesso: `C1 05 40 idx` ao alvo.
- `41` (C3): `[3]` resposta, `[4..5]` quem pediu. Mapa sem party ⇒ `C1 04 41 00` a quem respondeu; recusa ⇒ `41 01` **a quem respondeu**; aceite cria a party (líder = quem pediu) e adiciona; falha ⇒ `41 02` aos dois. Pedido sem resposta em 5 s ⇒ `41 00` aos dois (`FUN_0050e280`).
- `42`: `C1 size 42 01 n` + 24 bytes/membro (nome 10, slot, mapa, X, Y, 2 bytes, vida u32, vida máx u32); sem party `C1 05 42 00 00`. Enviada a todos em cada criação/entrada/saída (`FUN_004d32d0`).
- `43`: `[3]` slot. O próprio membro ou o líder; com < 3 membros a party é desfeita (todos recebem `C1 03 43`), senão o membro sai (líder substituído pelo próximo).
- `44`: a cada segundo, por membro, a todos: `C1 size 44 n` + `(vida*10/vidaMáx)&0xF ^ slot<<4`.
- Saída do mundo (`FUN_004c4b40`): com `ReconnectTime ≥ 1` o membro sai enquanto restar mais de um; o último desfaz.

## 5. PK

- `obj+0x19c` nível (0..2 herói, 3 comum, 4 aviso, 5..6 assassino), `+0x1a0` contagem, `+0x1a4` tempo em pontos.
- A cada segundo `FUN_0050e750(PKDownPlusTimePoint)`: assassinos sempre perdem pontos; o nível segue os limiares `> Req2 ⇒ 6/0`, `> Req1 ⇒ 5/1`, `> 0 ⇒ 4/2`, senão 3.
- Morte de jogador por jogador (`FUN_0050f330`): ignora Custom Arena, Chaos Castle (mapas 18..23), Castle Siege com vítima > 5, mapa com `NonPK = 2` e legítima defesa (vítima atacou primeiro, `SelfDefenseTime`). Comum/herói matando assassino (> 4) vira/continua herói (`PKTime = Req1/Req2/Req3` ou `+= Req4`); senão vira 3 com contagem 1 e `PKTime = Req1` ⇒ 4. Assassino matando não-assassino: contagem+1 e `PKTime = Req2/Req3` ou `+= Req4`.
- Pacote `C1 07 F3 08 idx(BE) nível` a si e ao viewport (`FUN_004db9d0`).

## 6. Incógnitas

| Id | Descrição |
|---|---|
| U-STATS-01 | Opções de set ancient (`FUN_004f2f70`, `FUN_004f30f0`, SetItemOption.txt) não reproduzidas. |
| U-STATS-02 | Campos de efeitos/buffs/master skill do CalcCharacter e do `F3 E1` (`+0x2a0`, `+0x298`, `+0x710..+0x800`, `FUN_004fe430/500/630`) ficam 0 até existir o módulo de efeitos. |
| U-STATS-03 | Nível de pet (`CItem+0xc4`) vem do DataServer (PetItemInfo) e não é persistido ⇒ 0. |
| U-PARTY-01 | Auto-party por senha (`FUN_004d2780`, `obj+0x434` bit 2, comandos custom). |
| U-PARTY-02 | Restauração da party ao reconectar (`FUN_004e1bb0`). |
| U-PK-01 | Duelo, guerra de guilds e regras completas de Castle Siege na morte por jogador. |
