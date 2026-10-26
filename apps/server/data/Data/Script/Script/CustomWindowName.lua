--############################################################################
-- SSeMU MU Online Emulator
-- https://www.ssemu.com.ar
-- https://www.facebook.com/ssemuemulator
-- https://www.youtube.com/@ssemu
-- ---------------------------------------------------------------------------
-- © 2023 Creado por ponneyhd, Modificado por Anthony Rosas
-- Description: Este archivo es un script para poder ver cierta informacion en el titulo de la ventana de window
--############################################################################

-- Enlazamos las funciones bridge a nuestras custom function
-- ===========================================================================
BridgeFunctionAttach('OnTimerThread', 'CustomWindowName_OnTimerThread')
-- ===========================================================================

-- Se ejecuta en un hilo continuamente
function CustomWindowName_OnTimerThread()

    -- Iteramos desde el usuario minimo hasta el usuario maximo del servidor
    for i = GetMinUserIndex(), GetMaxUserIndex(), 1 do

        -- Obtenemos la data de la cuenta
        local UserName = GetObjectName(i) or ''
        local UserLevel = GetObjectLevel(i) or 0
        local UserReset = GetObjectReset(i) or 0
        local UserAccountLevel = GetObjectAccountLevel(i) or ''

        -- Guardamos el nivel de vip de la cuenta
        if (UserAccountLevel == 0) then
            UserAccountLevel = 'Free'
        end
        if (UserAccountLevel == 1) then
            UserAccountLevel = 'VIP'
        end
        if (UserAccountLevel == 2) then
            UserAccountLevel = 'VIP'
        end
        if (UserAccountLevel == 3) then
            UserAccountLevel = 'VIP'
        end

        -- Ponemos en el titulo
        SetObjectWindowTitle(i, string.format("MuJogatina 97D - [%s] [%s] - Resets: %d - Level: %d", UserName, UserAccountLevel, UserReset, UserLevel))
    end
end
