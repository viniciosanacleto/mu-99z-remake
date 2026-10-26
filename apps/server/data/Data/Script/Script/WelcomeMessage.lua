-- ===========================================================================
BridgeFunctionAttach('OnCharacterEntry','WelcomeMessage_OnCharacterEntry')
BridgeFunctionAttach('OnCharacterClose','WelcomeMessage_OnCharacterClose')
-- ===========================================================================

-- Bảng giới hạn reset theo ngày trong tuần (0=Sunday,...6=Saturday)
local ResetLimits = {
    [0] = 25, -- Sunday
    [1] = 20, -- Monday
    [2] = 20, -- Tuesday
    [3] = 20, -- Wednesday
    [4] = 20, -- Thursday
    [5] = 20, -- Friday
    [6] = 25  -- Saturday
}

-- Biến lưu số lần reset hôm nay của người chơi, key = aIndex
local UserResetData = {}

-- Biến lưu trạng thái bonus GHRS hôm nay của người chơi
local GHRSBonusToday = {}

function GetMaxResetToday(aIndex)
    local wday = os.date("*t").wday - 1
    return ResetLimits[wday] or 20
end

function GetUserResetToday(aIndex)
    return UserResetData[aIndex] or 0
end

function IsGHRSBonusToday(aIndex)
    return GHRSBonusToday[aIndex] == true
end

function UpdateUserReset(aIndex, resetCount)
    UserResetData[aIndex] = resetCount
end

function SetUserGHRSBonus(aIndex, enabled)
    GHRSBonusToday[aIndex] = enabled and true or false
end

function WelcomeMessage_OnCharacterEntry(aIndex)

	local UserName = GetObjectName(aIndex)

	local UserAccountLevel = GetObjectAccountLevel(aIndex)

	local UserAccountExpireDate = GetObjectAccountExpireDate(aIndex)

	-- ==== PHẦN THÔNG BÁO GHRS =====
	local MaxReset = GetMaxResetToday(aIndex)
	local UserResetToday = GetUserResetToday(aIndex)
	local GHRSBonus = IsGHRSBonusToday(aIndex)

	NoticeSend(aIndex, 0, string.format(MessageGet(990), MaxReset))

	if UserResetToday >= MaxReset then
		NoticeSend(aIndex, 1, string.format(MessageGet(991), UserResetToday))
	end

	if GHRSBonus then
		NoticeSend(aIndex, 1, string.format(MessageGet(992), MaxReset))
	end
	-- ============================

	NoticeSend(aIndex,0,string.format(MessageGet(154),UserName))

	if UserAccountLevel == 0 then
		NoticeSend(aIndex,1,string.format(MessageGet(155),UserAccountExpireDate))
	elseif UserAccountLevel == 1 then
		NoticeSend(aIndex,1,string.format(MessageGet(156),UserAccountExpireDate))
	elseif UserAccountLevel == 2 then
		NoticeSend(aIndex,1,string.format(MessageGet(157),UserAccountExpireDate))
	elseif UserAccountLevel == 3 then
		NoticeSend(aIndex,1,string.format(MessageGet(158),UserAccountExpireDate))
	end

	local GameMasterLevel = CommandCheckGameMasterLevel(aIndex,8)
	if GameMasterLevel == 1 then
		NoticeSendToAll(0,string.format("GameMaster %s Online!",UserName))
	end

	MessageSend(aIndex,2,2,"Bem-vindo ao MU Online")
	MessageSend(aIndex,2,2,"Ajude-nos a crescer")
	MessageSend(aIndex,2,2,"/cemiterio /bar /pista /offline /bau1 ..20")

end

function WelcomeMessage_OnCharacterClose(aIndex)

	local UserName = GetObjectName(aIndex)

	local GameMasterLevel = CommandCheckGameMasterLevel(aIndex,8)

	if GameMasterLevel == 1 then

		NoticeSendToAll(0,string.format("GameMaster %s Offline!",UserName))

	end
end
