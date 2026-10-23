# Plano de reconstrução — Cliente

Especificação técnica para reescrever o cliente MU Online "97D/99z" como aplicação **multiplataforma (Windows, macOS, Linux)**, compatível com o protocolo do servidor original e capaz de usar os assets originais.

> **Decisão pendente**: a linguagem/engine do cliente ainda não foi escolhida (ver `CLAUDE.md`). Estes documentos são deliberadamente agnósticos de engine; [01-arquitetura-alvo.md](01-arquitetura-alvo.md) lista os requisitos que a escolha precisa atender.

Mesmas convenções de evidência do plano do servidor: **observado** (lido no binário/arquivos), **inferido** (dedução a validar), **desconhecido** (em [06-roadmap-e-incognitas.md](06-roadmap-e-incognitas.md)).

## Artefatos analisados

| Arquivo | Tipo | Papel | SHA-256 (prefixo) |
|---|---|---|---|
| `Cliente/Main.exe` | PE32 x86 GUI, 2,9 MB, 5892 funções, referência a `main.pdb` | Cliente Webzen original (renderer OpenGL, lógica, rede) | `ad96a79f…` |
| `Cliente/Main.dll` | PE32 DLL x86, 1,6 MB | Hook da WizardTeam: customizações, criptografia, patch de IP/versão/serial, proteção | `dbfacef8…` |
| `Cliente/opengl32.dll` | PE32 DLL | Wrapper "QeffectsGL" de pós-processamento (`QeffectsGL.ini`) | não analisado |
| `wzaudio.dll`, `ogg.dll`, `vorbisfile.dll` | DLLs | áudio (WAV + Ogg Vorbis) | não analisado |
| `License.wzt` | dados cifrados | configuração do `Main.dll` gerada por `GetMainInfo.exe` a partir de `PATCH CLIENTE/MainInfo.ini` e `PATCH CLIENTE/Data/Custom*.txt` | — |
| `Cliente/Data/**` | ~6000 arquivos, 494 MB (com servidor) | modelos `.bmd`, texturas `.OZJ/.OZT/.OZB`, terrenos `.map/.att/.obj`, sons `.wav`, tabelas `.bmd` | — |

## Composição do cliente original

```
Main.exe ──carrega──► Main.dll (WizardTeam)
   │                    ├─ lê License.wzt (IP, porta, versão, serial, chaves de atalho, flags custom)
   │                    ├─ CPacketManager (criptografia), CProtect
   │                    └─ CCustom* (itens, asas, joias, pets, mapas, monstros, tooltips, títulos, fog, efeitos, câmera 3D)
   ├── opengl32.dll (proxy de pós-processamento)
   └── Data\ (assets)
```

## Índice

| Documento | Conteúdo |
|---|---|
| [01-arquitetura-alvo.md](01-arquitetura-alvo.md) | Requisitos, camadas, decisões a tomar (engine), estrutura de `apps/client` |
| [02-formatos-de-assets.md](02-formatos-de-assets.md) | Formatos binários: OZJ/OZT/OZB, BMD, terreno (`.map/.att/.obj`), tabelas `Item.bmd`, `Gate.bmd`, `Skill.bmd`…, criptografias de arquivo |
| [03-rede-e-protocolo.md](03-rede-e-protocolo.md) | Lado cliente do protocolo: conexão, criptografia de envio, versão/serial, ordem dos pacotes por cena |
| [04-cenas-e-interface.md](04-cenas-e-interface.md) | Cenas (logo, login, servidores, personagens, jogo), janelas de UI, texturas de interface, teclas, opções |
| [05-renderizacao-e-mundo.md](05-renderizacao-e-mundo.md) | Terreno, objetos, modelos/animação esquelética, efeitos, câmera, pós-processamento, áudio |
| [06-roadmap-e-incognitas.md](06-roadmap-e-incognitas.md) | Fases e incógnitas abertas |

Referências cruzadas ao servidor: protocolo completo em [../server-plan/02-protocolo-de-rede.md](../server-plan/02-protocolo-de-rede.md); formato de item em [../server-plan/09-itens-e-inventario.md](../server-plan/09-itens-e-inventario.md).
