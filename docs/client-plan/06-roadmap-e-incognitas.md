# 06 — Roadmap e incógnitas do cliente

## 1. Fases

| Fase | Entrega | Critério de pronto |
|---|---|---|
| **0 — Decisão de engine** | Escolha registrada no `CLAUDE.md` com base nos critérios de [01 §3](01-arquitetura-alvo.md#3-decisão-de-enginelinguagem-pendente--confirmar-com-o-usuário) | Protótipo "hello window" nos 3 SOs |
| **1 — Assets** | Biblioteca de decoders (OZJ/OZT/OZB, BMD, `.map/.att/.obj`, `Item.bmd`, `Gate.bmd`) + visualizador de modelos/terreno + exportador glTF | Abre 100% dos arquivos de `Data/` sem erro; Lorencia renderizada com objetos |
| **2 — Rede/login** | Cliente headless + cenas Login/Seleção | Login no servidor original **e** no servidor Go; lista/cria/apaga personagem |
| **3 — Mundo** | Entrar no mundo, andar (clique + pathfinding nos `.att`), ver outros jogadores/monstros (viewport), chat | Dois clientes novos se veem; cliente novo convive com cliente original no mesmo servidor |
| **4 — Combate e itens** | Ataques, skills e efeitos, HUD, inventário, lojas, baú | Fluxo de caça completo |
| **5 — Sistemas** | Party, guild, trade, loja pessoal, amigos/mail, quests, eventos, Castle Siege UI | Paridade funcional com o original |
| **6 — Customs e polimento** | Equivalentes ao `Main.dll` priorizados; pós-processamento; launcher | — |

### Assets

Licenças dos arquivos originais são ignoradas neste projeto. Quando uma fase precisar de assets, copiá-los de `Cliente/Data/**` para `apps/client/assets/`, mantendo caminhos e nomes originais (ex. `apps/client/assets/World1/EncTerrain1.map`), e copiar só o que a tarefa usa. O cliente carrega os assets dessa pasta do repositório.

## 2. Estratégia de verificação

- Testes de decoder com arquivos reais (hash do resultado decodificado guardado como golden).
- Comparação visual lado a lado com o cliente original (screenshots na mesma posição/câmera; `ScreenShots/` do original como referência).
- Capturas de rede do cliente original (o mesmo proxy da fase 0 do servidor) para garantir que o cliente novo envia bytes idênticos.

## 3. Incógnitas abertas

| ID | Pergunta | Como resolver |
|---|---|---|
| U-ASSET-01 | BMD versões `0x0E` e `0x00` são carregadas? | Checar quem referencia esses arquivos; tentar parse |
| U-ASSET-02 | Significado byte a byte de `Vertex`/`Normal`/`Triangle` no BMD | Decompilar renderer de malha |
| U-ASSET-03 | Escala da altura (`TerrainHeight`) e tamanho da célula em unidades de mundo | `FUN_005e7690`, `FUN_005ea8e0` |
| U-ASSET-04 | Layout de `Skill.bmd` (60 bytes) | Loader `FUN_0053f810` + comparação com `SkillList.txt` |
| U-ASSET-05 | Formato de `Text.bmd`, `Dialog.bmd`, `Quest.bmd`, `Slide.bmd`, `Movereq.bmd`, `Filter*.bmd`, `ItemSetOption/Type.bmd` | Loaders em `FUN_006096b0` (`0x53e7c0`, `0x53eb00`, `0x53fdd0`, `0x417240`, `0x465d20`, `0x4443e0`…) |
| U-ASSET-06 | Pasta `Data\Music` ausente nesta cópia — músicas existem? | Verificar instalação completa do cliente |
| U-ASSET-07 | Mapeamento índice de tile → textura | Renderer de terreno |
| U-NET-01 | Como o `Main.dll` substitui IP/versão/serial no `Main.exe` | Decompilar inicialização do `Main.dll` (`CProtect`, leitura de `License.wzt`) |
| U-NET-02 | Cifra XOR32 de envio exata | `CPacketManager` do `Main.dll` + captura |
| U-NET-03 | Onde o cliente aplica SimpleModulus e serial | idem |
| U-NET-04 | Pacotes de inventário/skills/estado após `F3 03` | Capturas + handlers do cliente |
| U-NET-05 | Comportamento de reconexão automática | `Main.dll` + servidor `CReconnect` |
| U-UI-01 | Lista completa de chaves de registro de opções | strings/`RegQueryValue` no `Main.exe` |
| U-RND-01 | Matriz de projeção/câmera e escala do mundo | renderer |
| U-RND-02 | Comportamentos especiais por tipo de objeto de mapa | código de objetos por mapa |
| U-RND-03 | Catálogo skill → efeito visual | código de efeitos |
| U-RND-04 | Parâmetros padrão da câmera | `CCamera` / câmera do `Main.exe` |
| U-LIC-01 | Formato de `License.wzt` (gerado por `GetMainInfo.exe` de `MainInfo.ini` + `Custom*.txt`) | Decompilar `GetMainInfo.exe` ou o leitor no `Main.dll` ("Could not load License.wzt!") |
