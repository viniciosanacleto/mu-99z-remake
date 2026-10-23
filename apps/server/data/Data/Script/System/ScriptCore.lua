-- =============================================================================
-- # WizarD TeaM Emulator
-- # https://www.wzteam.com.br/
-- # https://www.facebook.com/WizardTeamDev
-- # https://www.youtube.com/@wizardteam9953
-- =============================================================================
-- # © 2008 - 2025 WizarD TeaM Development
-- # This file is part of the WzTeaM MuOnline Server files.
-- =============================================================================

-- ===========================================================================
-- ADVERTENCIA: Modificar este archivo puede afectar la funcionalidad del juego. 
-- Cualquier modificación incorrecta puede resultar en errores, fallos o comportamientos inesperados en el juego.

-- WARNING: Modifying this file may affect the functionality of the game.
-- Any incorrect modification can result in errors, crashes, or unexpected behavior in the game.
-- ===========================================================================

BridgeFunctionTable = {}

function BridgeFunctionAttach(BridgeName,FunctionName)
	
	if BridgeFunctionTable[BridgeName] == nil then
		
		BridgeFunctionTable[BridgeName] = {}
		
	end
	
	for _, func in ipairs(BridgeFunctionTable[BridgeName]) do
		
		if func.Function == FunctionName then
			
			return
			
		end
	end
	
	table.insert(BridgeFunctionTable[BridgeName], { Function = FunctionName })

end


function BridgeFunction_OnReadScript()

	if BridgeFunctionTable["OnReadScript"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnReadScript"]) do
		
			_G[func.Function]()
			
		end
		
	end

end


function BridgeFunction_OnShutScript()

	if BridgeFunctionTable["OnShutScript"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnShutScript"]) do
		
			_G[func.Function]()
			
		end
		
	end

end


function BridgeFunction_OnTimerThread()

	if BridgeFunctionTable["OnTimerThread"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnTimerThread"]) do
		
			_G[func.Function]()
			
		end
		
	end

end


function BridgeFunction_OnCommandManager(...)

	if BridgeFunctionTable["OnCommandManager"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnCommandManager"]) do
		
			local ret =_G[func.Function](...)
			
			if ret ~= 0 then return 1 end
			
		end
		
	end

	return 0

end


function BridgeFunction_OnCommandDone(...)

	if BridgeFunctionTable["OnCommandDone"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnCommandDone"]) do
		
			_G[func.Function](...)
			
		end
		
	end

end


function BridgeFunction_OnCharacterEntry(...)

	if BridgeFunctionTable["OnCharacterEntry"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnCharacterEntry"]) do
		
			_G[func.Function](...)
			
		end
		
	end

end


function BridgeFunction_OnCharacterClose(...)

	if BridgeFunctionTable["OnCharacterClose"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnCharacterClose"]) do
		
			_G[func.Function](...)
			
		end
		
	end

end


function BridgeFunction_OnNpcTalk(...)

	if BridgeFunctionTable["OnNpcTalk"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnNpcTalk"]) do
		
			local ret =_G[func.Function](...)
			
			if ret ~= 0 then return 1 end
			
		end
		
	end

	return 0

end


function BridgeFunction_OnMonsterDie(...)

	if BridgeFunctionTable["OnMonsterDie"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnMonsterDie"]) do
		
			_G[func.Function](...)
			
		end
		
	end

end


function BridgeFunction_OnUserDie(...)

	if BridgeFunctionTable["OnUserDie"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnUserDie"]) do
		
			_G[func.Function](...)
			
		end
		
	end

end

function BridgeFunction_OnUserMove(...)

	if BridgeFunctionTable["OnUserMove"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnUserMove"]) do
		
			_G[func.Function](...)
			
		end
		
	end

end

function BridgeFunction_OnUserRespawn(...)

	if BridgeFunctionTable["OnUserRespawn"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnUserRespawn"]) do
		
			_G[func.Function](...)
			
		end
		
	end

end


function BridgeFunction_OnCheckUserTarget(...)

	if BridgeFunctionTable["OnCheckUserTarget"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnCheckUserTarget"]) do
		
			local ret =_G[func.Function](...)
			
			if ret == 0 then return 0 end
			
		end
		
	end

	return 1

end


function BridgeFunction_OnCheckUserKiller(...)

	if BridgeFunctionTable["OnCheckUserKiller"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnCheckUserKiller"]) do
		
			local ret =_G[func.Function](...)
			
			if ret == 0 then return 0 end
			
		end
		
	end

	return 1

end


function BridgeFunction_OnUserItemPick(...)

	if BridgeFunctionTable["OnUserItemPick"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnUserItemPick"]) do
		
			local ret =_G[func.Function](...)
			
			if ret == 0 then return 0 end
			
		end
		
	end

	return 1

end


function BridgeFunction_OnUserItemDrop(...)

	if BridgeFunctionTable["OnUserItemDrop"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnUserItemDrop"]) do
		
			local ret =_G[func.Function](...)
			
			if ret == 0 then return 0 end
			
		end
		
	end

	return 1

end


function BridgeFunction_OnUserItemMove(...)

	if BridgeFunctionTable["OnUserItemMove"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnUserItemMove"]) do
		
			local ret =_G[func.Function](...)
			
			if ret == 0 then return 0 end
			
		end
		
	end

	return 1

end

function BridgeFunction_OnSQLAsyncResult(...)

	if BridgeFunctionTable["OnSQLAsyncResult"] ~= nil then
	
		for _, func in ipairs(BridgeFunctionTable["OnSQLAsyncResult"]) do
		
			_G[func.Function](...)
			
		end
		
	end
	
end