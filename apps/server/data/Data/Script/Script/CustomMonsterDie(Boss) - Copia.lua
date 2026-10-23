-- Definição da tabela goldenMonsters
BridgeFunctionAttach("OnMonsterDie", "MonsterMessage_OnMonsterDie")
BridgeFunctionAttach("OnTimerThread", "OnTimerThread_InitGoldenEvent")

--[[ 
    Tudo configurável por aqui, marcão
    Pro sistema funcionar direitinho 
    Count tem que ser igual ao que você configurar no EventManager (No caso de invasões)
    killed manter sempre 0
--]]

local goldenMonsters = 
{
	[55]  = { Name = "Skeleton King", 		Count = 1,  killed = 0},
	[44]  = { Name = "Red Dragon", 			Count = 3,  killed = 0},
	[43]  = { Name = "Golden Budge Dragon", Count = 5,  killed = 0},
    [53]  = { Name = "Golden Titan", 		Count = 5,  killed = 0},
    [54]  = { Name = "Golden Soldier", 		Count = 5,  killed = 0},
    [78]  = { Name = "Golden Goblin", 		Count = 5,  killed = 0},
    [79]  = { Name = "Golden Derkon", 		Count = 3,  killed = 0},
    [81]  = { Name = "Golden Vepar", 		Count = 5,  killed = 0},
	[80]  = { Name = "Golden Lizard King", 	Count = 3,  killed = 0},
    [82]  = { Name = "Golden Tantallos", 	Count = 3,  killed = 0},
    [83]  = { Name = "Golden Wheel", 		Count = 5,  killed = 0},
	[135] = { Name = "White Wizard", 		Count = 1,  killed = 0},
	[25]  = { Name = "Ice Queen", 			Count = 10, killed = 0},
	[18]  = { Name = "Gorgon", 				Count = 10, killed = 0},
	[49]  = { Name = "Hydra", 				Count = 1,  killed = 0},
	[59]  = { Name = "Zaikan", 				Count = 10, killed = 0},
	[38]  = { Name = "Balrog", 				Count = 10, killed = 0},

}

local isGoldenEventActive = false



function OnTimerThread_InitGoldenEvent()
    local currentTime = os.date("*t")
    local currentHour = currentTime.hour
    local currentMinutes = currentTime.min
    local currentSeconds = currentTime.sec

    local EventSchedule = 
    {
		SkeletonKingStart = 
		{	
			{Hour = 02, Minutes = 05, Seconds = 00},
			{Hour = 08, Minutes = 05, Seconds = 00},
			{Hour = 16, Minutes = 05, Seconds = 00},	
		    {Hour = 20, Minutes = 05, Seconds = 00},
		},
       
	    RedDragonStart = 
        {
            {Hour = 09, Minutes = 30, Seconds = 00},
            {Hour = 13, Minutes = 30, Seconds = 00},
            {Hour = 17, Minutes = 30, Seconds = 00},
            {Hour = 21, Minutes = 30, Seconds = 00}
        },
        
		GoldenStart = 
		{

			{Hour = 2, Minutes = 00, Seconds = 00},
			{Hour = 6, Minutes = 00, Seconds = 00},
			{Hour = 10, Minutes = 15, Seconds = 00},
			{Hour = 14, Minutes = 15, Seconds = 00},

		},
		
		WhiteWizardStart =
		{
			{Hour = 05, Minutes = 00, Seconds = 00},
			{Hour = 09, Minutes = 00, Seconds = 00},
			{Hour = 15, Minutes = 00, Seconds = 00},
			{Hour = 19, Minutes = 00, Seconds = 00},
		},
		
        IceQueenStart = 
        {
            {Hour = 00, Minutes = 00, Seconds = 00},
            {Hour = 04, Minutes = 00, Seconds = 00},
            {Hour = 08, Minutes = 30, Seconds = 00},
            {Hour = 12, Minutes = 30, Seconds = 00}
        },
        
        DeathGorgonStart = 
        {
            {Hour = 20, Minutes = 00, Seconds = 00},
            {Hour = 22, Minutes = 00, Seconds = 00},
            {Hour = 00, Minutes = 00, Seconds = 00},
            {Hour = 03, Minutes = 00, Seconds = 00}
        },
		
		HydraStart =
		{
			{Hour = 01, Minutes = 00, Seconds = 00},
			{Hour = 05, Minutes = 00, Seconds = 00},
			{Hour = 10, Minutes = 00, Seconds = 00},
			{Hour = 16, Minutes = 00, Seconds = 00},
		},

		ZaikanStart =
		{
			{Hour = 03, Minutes = 30, Seconds = 00},
			{Hour = 06, Minutes = 00, Seconds = 00},
			{Hour = 15, Minutes = 30, Seconds = 00},
			{Hour = 21, Minutes = 00, Seconds = 00},
		
		},
		
		BalrogStart = 
        {
            {Hour = 19, Minutes = 00, Seconds = 00},
            {Hour = 21, Minutes = 00, Seconds = 00},
            {Hour = 23, Minutes = 30, Seconds = 00},
            {Hour = 02, Minutes = 30, Seconds = 00}
        }



    }
    

    for period, events in pairs(EventSchedule) do
        for i = 1, #events do
            local event = events[i]
            
            if currentHour == event.Hour and currentMinutes == event.Minutes and currentSeconds == event.Seconds then
                
                if not isGoldenEventActive then
				
					if period == "SkeletonKingStart" then
						Warnings(period, event)
						isGoldenEventActive = true
					end
					
                    if period == "RedDragonStart" then
                        Warnings(period, event)
                        isGoldenEventActive = true
                    end
					
					if period == "GoldenStart" then
						Warnings(period, event)
					    isGoldenEventActive = true
					end
					
					if period == "WhiteWizardStart" then
						Warnings(period, event)
						isGoldenEventActive = true
					end				
					
					if period == "IceQueenStart" then
						Warnings(period, event)
						isGoldenEventActive = true
					end
					
					if period == "DeathGorgonStart" then
						Warnings(period, event)
						isGoldenEventActive = true
					end
					
					if period == "HydraStart" then
						Warnings(period, event)
						isGoldenEventActive = true
					end
										
					if period == "ZaikanStart" then
						Warnings(period, event)
						isGoldenEventActive = true
					end
					
					if period == "BalrogStart" then
						Warnings(period, event)
					    isGoldenEventActive = true
					end

                end 

                break
            else
                isGoldenEventActive = false
            end
        end
    end
end

-- Função para ser chamada quando um monstro morre
function MonsterMessage_OnMonsterDie(aIndex, bIndex)
    local MonsterClass = GetObjectClass(aIndex)
    local CharacterName = GetObjectName(bIndex)


    if goldenMonsters[MonsterClass] then
        goldenMonsters[MonsterClass].killed = goldenMonsters[MonsterClass].killed + 1
        local KilledMonsters = goldenMonsters[MonsterClass].killed
        local totalCount = goldenMonsters[MonsterClass].Count
		
		
		
		
	
		local bossMonsters = {[55] = true,[42] = true,[43] = true,[53] = true,[54] = true,[78] = true,[79] = true,[80] = true,[81] = true,[82] = true,[83] = true,[135] = true,[25] = true,[81] = true,[49] = true,[59] = true,[38] = true,}
			if bossMonsters[MonsterClass] then
				NoticeSendToAll(0, string.format("Dead Boss %s. [%d / %d]", goldenMonsters[MonsterClass].Name, KilledMonsters, totalCount))
			else
				NoticeSendToAll(0, string.format("%s matou um %s. [%d / %d]", CharacterName, goldenMonsters[MonsterClass].Name, KilledMonsters, totalCount))
			end
    end
end

function Warnings(period, event)

	if period == "SkeletonKingStart" then
		if goldenMonsters[55].killed ~= 0 then
			goldenMonsters[55].killed = 0
		end
    end
	
	if period == "RedDragonStart" then
		if goldenMonsters[42].killed ~= 0 then
			goldenMonsters[42].killed = 0
		end
	end
	
	if period == "GoldenStart" then
			goldenMonsters[43].killed = 0
			goldenMonsters[53].killed = 0
			goldenMonsters[54].killed = 0
			goldenMonsters[78].killed = 0
			goldenMonsters[79].killed = 0
			goldenMonsters[80].killed = 0
			goldenMonsters[81].killed = 0
			goldenMonsters[82].killed = 0
			goldenMonsters[83].killed = 0
		
	end
	
	if period == "WhiteWizardStart" then
		if goldenMonsters[135].killed ~= 0 then
		goldenMonsters[135].killed = 0
		end
    end
	
	if period == "IceQueenStart" then
		if goldenMonsters[25].killed ~= 0 then
			goldenMonsters[25].killed = 0
		end
	end
	
	if period == "DeathGorgonStart" then
		if goldenMonsters[81].killed ~= 0 then
			goldenMonsters[81].killed = 0
		end
	end
	
	if period == "HydraStart" then	
		if goldenMonsters[49].killed ~= 0 then
			goldenMonsters[49].killed = 0
		end
	end
	
	if period == "ZaikanStart" then	
		if goldenMonsters[59].killed ~= 0 then
			goldenMonsters[59].killed = 0
		end	 
	end
	if period == "BalrogStart" then
		if goldenMonsters[38].killed ~= 0 then
			goldenMonsters[38].killed = 0
		end
	end
end