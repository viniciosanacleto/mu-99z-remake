# 04 — Cenas e interface

## 1. Cenas (observado)

Strings de log do `Main.exe`: `> Login Scene init success.`, `> Login Scene - Exit`, `> Character scene init success.`, `> Main Scene init success.`. Fluxo:

| Cena | Assets (observados) | Pacotes |
|---|---|---|
| Logo/Login | `Data\Logo\` — `back1..4.jpg`, `bkcloud.jpg`, `bkmoon.jpg`, `cloud.jpg`, `Interface01..04.tga`, `0Box.jpg`, `0Text_Box.jpg`, `0On_Botton*.jpg`, `0Account_new.tga`, `0New_Account01/02.tga`, `Ok01/02`, `Delete01/02`; `Data\Local\Loading01..03.OZJ`, `webzenlogo.ozj`, `everyone.ozj`; custom `Custom\Logo\0Account_new.tga` | CS: lista/seleção; GS: `F1 00`, `F1 01` |
| Seleção/criação de personagem | `Logo\New_Character00x.jpg`, `New_Character01/02/201/202`, `New_Character_Ok/Cancel.jpg` | `F3 00/01/02/03` |
| Jogo (Main) | interface abaixo + mundo | todos |

Áudio de UI: `iTitle.wav`, `iButtonMove/Click/Error.wav`, `iCreateWindow.wav`, `iRepair.wav`, `iWhisper.wav`, `iFMSGAlert.wav`, `iFMailAlert.wav`, `iFLogInAlert.wav`, `iEvent3min/Start/End.wav`.

## 2. Texturas de interface do jogo (observado, `FUN_006096b0`)

Cursores (`Interface\Cursor*.tga`: normal, Push, Attack, Attack2, Get, Talk, Repair, LeanAgainst, SitDown, DontMove, ID), barra principal (`Menu01_new`, `Menu02`, `Menu03_new`, `Menu03_new_AG`, `Menu04`, `Menu_Red/Blue/Green` = HP/MP/AG), janelas (`Menu_Party`, `Menu_Character`, `Menu_Inventory`, `dialogue1/2`, `Message_box*`, `ok/ok2`, `cancel/cancel2`, `Level_box`, `level01`, `Guild`, `Quest1/2`, `pet1/2`, `setitem`, `btn_pshop*`, `text_button`, `DuelWindow`, `StatusIcon`, `Progress*`), janelas genéricas (`win_titlebar`, `win_button`, `win_size`, `win_resize`, `win_scrollbar`, `win_check`, `win_mail`, `win_mark`, `win_letter`, `win_man`, `win_push`, `win_question`), chat/NIS (`nis_rsframe`, `nis_vframe`, `nis_bar`, `nis_back`, `nis_btnarrow/chat/size/alpha`), Castle Siege (`senatusmap`, `gate_button1/2`, `suho_button1/2`, `DoorCL/OP`, `frame`, `i_attack/defense/wait`, `b_command01/02`, `b_group02`, `b_zoomout01`, `hourglass`, `dot`), slots de equipamento (`Item_*`), números (`Font*.ozt`).

Customizações do `Main.dll` sobrepõem: `Custom\Interface\Menu01_new.jpg`, `Menu03_new.jpg`, `Menu_Red/Blue/Green.jpg`, `TB_Left_1/2.tga`, `TB_Right_1/2.tga`, botões `Custom\Buttons\hp_on/off.jpg`, `whare_on/off.jpg` (`ClientButtons.txt`), ícone `Data\Custom\main.ico`, mapa `Data\World%d\Map1.ozj` (minimapa).

## 3. Janelas e funcionalidades a implementar

Derivadas dos opcodes do servidor ([../server-plan/02 §7](../server-plan/02-protocolo-de-rede.md#7-tabela-de-opcodes-cliente--gameserver-observado)) e das texturas acima:

- HUD: HP/MP/AG, experiência, skill ativa, atalhos (`OptionData`: SkillKey 1–0, Q/W/E de poções), chat com janelas/filtros (`ChatWindow`), notícias/slide (`Slide.bmd`), mensagens de evento.
- Inventário 8×8 + equipamento, personagem (stats e distribuição de pontos), skills, party, guild (logo 8×8, alianças), amigos/mail, loja de NPC, trade, loja pessoal, baú (+ baús extras), Chaos Machine, quest, duelo, janela de movimentação (`Movereq.bmd`, `/move`), mapa/minimapa, opções.
- Custom (`MainInfo.ini [CustomInfo]`): menu de opções avançado, Golden Archer renovado, mover item com clique direito, minimapa, janela de horários de eventos, "recuo", logo de guild, skills de cerco, posição da barra de HP, Jewel Bank, troca de baú, limpar inventário, armas+escudo, downgrade, transparência; X-Shop (cash shop com recarga PIX); títulos de item (`CustomItemTitle`), tooltips (`CustomTooltip`), barra de HP de monstros (F9), auto-attack (F8), câmera 3D (F10/F11), modo bandeja (F12).

## 4. Opções e persistência local

- Registro do Windows `HKCU\Software\Webzen\Mu\Config`: `ID` (lembrar conta), `Resolution`, `WindowMode`, `ScreenWidth/Height`, `FullScreen`, `MusicOnOff` (e outros a levantar — U-UI-01). Na reescrita: arquivo de configuração por usuário (XDG/AppData/Library).
- `Config.ini [FontInfo]`: `ChangeFontSwitch`, `FontName`, `FontSize`, `FontCharSet`.
- `Data\Macro.txt`: macros.
- Screenshots: `ScreenShots\Screen(%02d_%02d-%02d-%02d)-%04d.jpg`.

## 5. Launcher (observado, `LauncherOption.ini`)

Launcher externo (não incluso) com auto-update por manifesto HTTP (`BaseUrl`, `Manifest=manifest.txt`), notícias (`News.Url`), links, e gravação das opções de vídeo no registro. Na reescrita: launcher/updater multiplataforma separado do jogo (fora do escopo inicial).
