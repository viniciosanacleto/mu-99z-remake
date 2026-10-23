# 01 — Arquitetura alvo do cliente

## 1. Requisitos (derivados do original e do objetivo do projeto)

| # | Requisito | Origem |
|---|---|---|
| R1 | Rodar em Windows, macOS e Linux | `CLAUDE.md` |
| R2 | Falar o protocolo do servidor original byte a byte (C1–C4, XOR32, SimpleModulus, Bux) | compatibilidade com servidores existentes e com a reescrita Go |
| R3 | Ler os assets originais **sem conversão obrigatória** (BMD, OZJ/OZT/OZB, `.map/.att/.obj`, tabelas `.bmd`), com opção de pipeline de conversão offline para formatos modernos (glTF/KTX2/PNG) | reaproveitar 400+ MB de conteúdo |
| R4 | Mundo 256×256 células por mapa, câmera isométrica fixa com zoom/rotação limitados (+ câmera 3D livre, feature custom `KeyCodeCamera3DSwitch`) | `Main.exe`, `MainInfo.ini` |
| R5 | Animação esquelética por keyframes (posição + rotação por bone por frame) | formato BMD |
| R6 | Resolução configurável (o original grava em `HKCU\Software\Webzen\Mu\Config`: `Resolution`, `WindowMode`, `ScreenWidth/Height`), modo janela | `LauncherOption.ini` |
| R7 | Fonte configurável (`Config.ini`: `FontName = Tahoma`, `FontSize = 13`) e IME (o original usa `Imm*` para entrada de texto asiático) | `Main.exe` imports |
| R8 | Idiomas Eng/Por/Spn (`Data/Local/<lang>/*_<lang>.bmd`) | `Main.dll` |
| R9 | Áudio: efeitos WAV 3D/2D e música (Ogg via `vorbisfile.dll`; `Data\Music\%s`) | `Main.exe` strings |
| R10 | Customizações WizardTeam como módulos opcionais (asas/itens/joias/pets/mapas custom, tooltips, títulos, barra de HP, auto-attack, jewel bank, X-Shop…) | `Main.dll` |

## 2. Camadas

```
apps/client/
  platform/       janela, input, áudio, filesystem, relógio (abstração da engine/SO)
  assets/         decoders: ozj, ozt, ozb, bmd (modelo+anim), terrain(map/att/obj), tables(item/skill/gate/text/dialog/quest/movereq/slide/filter), crypto(MapFileDecrypt, Bux)
  net/            frame C1–C4, XOR32 de envio, SimpleModulus (Enc1/Dec2), serial, fila de pacotes, reconexão
  protocol/       structs por opcode (compartilhar a definição com o servidor Go via schema/gerador, se possível)
  game/           estado do mundo: personagem, viewport (objetos remotos), inventário, skills, party, guild, chat…
  scene/          Logo → Login → ServerSelect → CharacterSelect/Create → World
  render/         terreno, objetos estáticos, personagens (montagem por CharSet), efeitos/partículas, UI
  ui/             janelas (inventário, personagem, loja, trade, baú, guild, party, chat, mapa, opções…)
  custom/         módulos equivalentes ao Main.dll
```

Princípios:
- **Separar simulação de renderização**: `game/` não depende de `render/`; facilita testes e bots de teste para o servidor.
- **Decoders de assets como biblioteca independente** (com testes contra os arquivos reais), reutilizáveis por ferramentas de conversão e pelo servidor (o servidor precisa de `.att`).
- **Protocolo compartilhado**: idealmente as definições de pacote ficam em um formato neutro (ex. YAML/Protobuf-like) que gera Go (servidor) e a linguagem do cliente.

## 3. Decisão de engine/linguagem (pendente — confirmar com o usuário)

Critérios que a escolha precisa satisfazer, com base no que o original exige:

| Critério | Por quê |
|---|---|
| Export nativo para Win/macOS/Linux | R1 |
| Controle de baixo nível de sockets TCP e bytes | R2 (cripto custom, pacotes binários) |
| Carregamento de malhas/animação em runtime a partir de formato próprio | R3/R5 — BMD não é suportado por nenhuma engine; precisa de importador próprio (runtime ou offline → glTF) |
| Renderização de terreno em grade com 2 camadas de tile + alpha + lightmap | ver [05](05-renderizacao-e-mundo.md) |
| UI 2D com muitas janelas e texto (inclusive IME) | [04](04-cenas-e-interface.md) |

Opções típicas a avaliar (não decidido): engine completa (Godot/C#, Unity/C#, Unreal/C++), framework (Bevy/Rust, MonoGame/C#, raylib/C/Go, Ebitengine é 2D-only), ou C++ com SDL3 + bgfx/OpenGL. A decisão deve ser registrada no `CLAUDE.md` antes de iniciar `apps/client`.

## 4. Configuração do cliente

O original lê (via `Main.dll`/`License.wzt`) os campos de `PATCH CLIENTE/MainInfo.ini`:

| Seção | Chaves |
|---|---|
| `MainInfo` | `CustomerName`, `LauncherStart`, `LauncherMutex`, `IpAddress` (do ConnectServer), `IpAddressPort` (44405), `ClientVersion` (`1.00.00`), `ClientSerial` (16 chars), `WindowName`, `ScreenShotPath`, `ClientName`, `PluginName1..5` |
| `AnimationInfo` | `DW/DK/FE/MG/DLAttackAnimationValue` |
| `KeyInfo` | `KeyCodeAutoAttackSwitch` (119 = F8), `KeyCodeHealthBarSwitch` (120 = F9), `KeyCodeCamera3DSwitch` (121 = F10), `KeyCodeCamera3DRestore` (122 = F11), `KeyCodeTrayModeSwitch` (123 = F12) |
| `ReconnectInfo` | `ReconnectTime` (300000 ms) |
| `CustomInfo` | `AdvancedOptionMenuSwitch`, `GoldenArcherRenewalSwitch`, `ItemMoveRightClicSwitch`, `MiniMapSwitch`, `WindowEventTimeSwitch`, `EnableRecuo`, `EnableGuildLogo`, `EnableSkillSiege`, `EnableHPPosition`, `EnableJewelBank`, `EnableChangeWare`, `EnableClearInv`, `EnableWeapowsAndShield`, `EnableDowngrade`, `EnableTransparency`, `EnableBugTS` |
| `ResourceInfo` | `IncreaseFPSSwitch`, `ReduceConsumption` |

Recomendação: arquivo `client.toml` legível (sem `License.wzt`), com os mesmos campos.
