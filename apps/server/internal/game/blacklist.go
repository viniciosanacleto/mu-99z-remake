package game

// CBlackList of the GameServer (Data/BlackList.txt, DAT_012419c8, reader
// FUN_00411ae0): section 0 IP addresses, section 1 hardware ids, each kept
// in a std::set of strings. It is not the ConnectServer's own
// 1.ConnectServer/BlackList.txt (internal/connect, flag -blacklist of
// muserver), which only filters the ConnectServer port.
//
//   - IP: the GameServer socket accepts with WSAAccept and the condition
//     FUN_005054f0, which returns CF_REJECT when FUN_00411f00 finds
//     inet_ntoa(address) in section 0: the connection is refused before
//     any index is allocated or packet sent.
//   - HWID: F3 09 (FUN_00441730) closes the client (FUN_00512780) when
//     FUN_00411fa0 finds the 44-character hardware id in section 1.

// acceptFilterHook decides whether a new GameServer connection from ip is
// accepted (world.go).
var acceptFilterHook func(w *World, ip string) bool

func init() {
	acceptFilterHook = func(w *World, ip string) bool { return !w.blacklistedIP(ip) }
}

// blacklistedIP is the std::set lookup of FUN_00411f00 (exact string;
// the reader keeps at most 16 characters).
func (w *World) blacklistedIP(ip string) bool { return w.dus().blackIPs[ip] }

// blacklistedHWID is FUN_00411fa0.
func (w *World) blacklistedHWID(hwid string) bool { return w.dus().blackHWIDs[hwid] }
