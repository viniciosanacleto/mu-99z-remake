# 05 — Renderização, mundo e áudio

## 1. Renderer original (observado)

- OpenGL 1.x imediato (strings `glViewport`, `glFog*`, `OpenGL version : %s`, `Max Viewport size : %d x %d`), filtros `GL_NEAREST/LINEAR`, wrap `GL_CLAMP/REPEAT`.
- Áudio via `wzaudio.dll` + DirectSound/DirectMusic (`Couldn't create CLSID_DirectMusic`) e Ogg Vorbis.
- Pós-processamento por proxy `opengl32.dll` (QeffectsGL), configurado em `QeffectsGL.ini [main.exe]`: `RenderFrame`, `ZTrickFix`, `Anisotropy=20`, `Multisample=16`, `Bloom*`, `SSAO*`, `Hue/Saturation/Lightness`, `Emboss*`, `DOF*`, `MotionBlur`.

A reescrita deve usar uma API moderna (Vulkan/Metal/D3D12/GL3+ via engine) e oferecer esses efeitos como opções nativas.

## 2. Terreno

Dados em [02 §4](02-formatos-de-assets.md#4-terreno-observado):

- Grade 256×256; cada célula é um quad com duas camadas de tile (camada 2 misturada por alpha), altura do `TerrainHeight` e cor do lightmap `TerrainLight`.
- Tiles OZJ repetidos (`GL_REPEAT`); tiles OZT (grama) com transparência.
- Mapas com "céu"/skybox por faces (`World1/Back.bmp`, `Bottom`, `Front`, `Left`, `Right`, `Top`).
- Partículas de ambiente: folhas (`leaf01/02`), chuva (`rain01..03`, variante `rain011` condicionada por `FUN_0042e010`).
- Atributos `.att` usados para pathfinding local (clique para andar), cursor "DontMove" e zonas seguras.
- Escala mundo↔célula e fórmula da altura a extrair do renderer (U-RND-01).

## 3. Objetos estáticos

`.obj` ([02 §4.3](02-formatos-de-assets.md)) → instâncias de `Object{m+1}\ObjectNN.bmd` com posição, rotação (ângulos) e escala. Alguns objetos têm animação própria e efeitos (fogo, água, luz); o mapeamento "tipo de objeto → comportamento especial" está no código (U-RND-02).

## 4. Personagens e monstros

- Modelo do jogador = esqueleto da classe + partes de equipamento (`Player\HelmXXX`, `ArmorXXX`, `PantXXX`, `GloveXXX`, `BootXXX`) + arma(s) nas mãos + asas + pet, escolhidos a partir do **CharSet** de 18 bytes do viewport (layout a extrair — U-ITEM-03 no servidor). Cor/brilho por nível de item (+7..+15) e opções excelentes.
- Animações por ação (`18` action, ataques `D7`, skills); `AnimationInfo.<Classe>AttackAnimationValue` ajusta a ação de ataque (custom).
- Monstros/NPCs: `Monster\MonsterNN.bmd`, `NPC\*.bmd`; customs por `CustomMonster.txt` (`Scale`, `FolderPath`, `ModelName`, `Effect`).
- Barra de HP de monstro (custom, F9) com dados do servidor (`MonsterHealthBarSwitch`).

## 5. Efeitos e skills

Texturas `Effect\*` e `Skill\*` carregadas com ids fixos (ex. `0x5d9` Fire01, `0x5ca` Thunder01, `0x640` SwordEff…, lista completa em `FUN_006096b0`); modelos em `Skill\*.bmd`. Cada skill do `Skill.bmd` mapeia para um efeito visual no código (`CCustomEffect`/`cSmokeEffect` no `Main.dll` acrescentam efeitos). Catálogo skill→efeito a levantar (U-RND-03).

## 6. Câmera

Câmera padrão isométrica seguindo o personagem; `CCamera` (Main.dll) adiciona câmera 3D livre (F10) e restaurar (F11). Ângulos/distância padrão a extrair (U-RND-04).

## 7. Neblina e iluminação

`CCustomFog` + `PATCH CLIENTE/Data/CustomFog.txt` (neblina por mapa), `CustomSmoke.txt`, `CustomEffect.txt`. Lightmap estático por mapa + luzes dinâmicas de efeitos.

## 8. Áudio

- Ambiente por mapa (`aForest`, `aDungeon`, `aWater`, `aWind`, `aTower`, `aKalima*`, `aRain`, `aThunder*`…), efeitos de skill/combate (`e*`), monstros (`blood*`, `death*`…), UI (`i*`).
- Música por mapa (`Data\Music\%s`; `CustomMap.txt` permite `MusicPath` por mapa).
- Posicionamento 3D para efeitos no mundo.
